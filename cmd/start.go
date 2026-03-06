package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/peterbourgon/ff/v3/ffcli"
	"go.uber.org/zap"

	"github.com/gnolang/tx-indexer/client"
	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/fetch"
	"github.com/gnolang/tx-indexer/genesis"
	"github.com/gnolang/tx-indexer/serve"
	"github.com/gnolang/tx-indexer/serve/graph"
	"github.com/gnolang/tx-indexer/serve/handlers/supply"
	"github.com/gnolang/tx-indexer/serve/health"
	"github.com/gnolang/tx-indexer/storage"

	"github.com/gnolang/gno/tm2/pkg/std"
)

const (
	defaultRemote           = "http://127.0.0.1:26657"
	defaultDBPath           = "indexer-db"
	defaultCORSAllowOrigins = "*"
	// defaultSupplyDenoms is the denomination list the supply endpoint
	// tracks and refreshes in the background. The gas denom is what
	// aggregators ask for.
	defaultSupplyDenoms = "ugnot"
)

// corsAllowedOriginsHelp is built up over multiple lines so each stays under
// the linter's line-length limit.
const corsAllowedOriginsHelp = "a comma-separated list of origins allowed to make cross-origin requests " +
	"to the GraphQL and JSON-RPC endpoints, or \"*\" to allow any origin"

type startCfg struct {
	listenAddress        string
	remote               string
	dbPath               string
	logLevel             string
	corsAllowedOrigins   string
	supplyDenoms         string
	maxSlots             int
	maxChunkSize         int64
	rateLimit            int
	disableIntrospection bool
	clearOnReset         bool
}

// newStartCmd creates the indexer start command
func newStartCmd() *ffcli.Command {
	cfg := &startCfg{}

	fs := flag.NewFlagSet("start", flag.ExitOnError)
	cfg.registerFlags(fs)

	return &ffcli.Command{
		Name:       "start",
		ShortUsage: "start [flags]",
		ShortHelp:  "Starts the indexer service",
		LongHelp:   "Starts the indexer service, which includes the fetcher and JSON-RPC server",
		FlagSet:    fs,
		Exec: func(ctx context.Context, _ []string) error {
			return cfg.exec(ctx)
		},
	}
}

// registerFlags registers the indexer start command flags
func (c *startCfg) registerFlags(fs *flag.FlagSet) {
	fs.StringVar(
		&c.listenAddress,
		"listen-address",
		serve.DefaultListenAddress,
		"the IP:PORT URL for the indexer JSON-RPC server",
	)

	fs.StringVar(
		&c.remote,
		"remote",
		defaultRemote,
		"the JSON-RPC URL of the Gno chain",
	)

	fs.StringVar(
		&c.dbPath,
		"db-path",
		defaultDBPath,
		"the absolute path for the indexer DB (embedded)",
	)

	fs.StringVar(
		&c.logLevel,
		"log-level",
		zap.InfoLevel.String(),
		"the log level for the CLI output",
	)

	fs.IntVar(
		&c.maxSlots,
		"max-slots",
		fetch.DefaultMaxSlots,
		"the amount of slots (workers) the fetcher employs",
	)

	fs.Int64Var(
		&c.maxChunkSize,
		"max-chunk-size",
		fetch.DefaultMaxChunkSize,
		"the range for fetching blockchain data by a single worker",
	)

	fs.IntVar(
		&c.rateLimit,
		"http-rate-limit",
		0,
		"the maximum HTTP requests allowed per minute per IP, unlimited by default",
	)

	fs.BoolVar(
		&c.disableIntrospection,
		"disable-introspection",
		false,
		"disable GraphQL introspection queries if needed. This will cause malfunctions when using the GraphQL playground",
	)

	fs.StringVar(
		&c.corsAllowedOrigins,
		"cors-allowed-origins",
		defaultCORSAllowOrigins,
		corsAllowedOriginsHelp,
	)

	fs.StringVar(
		&c.supplyDenoms,
		"supply-denoms",
		defaultSupplyDenoms,
		"comma-separated denominations whose supply (total/spendable/locked) is tracked and served by getSupply",
	)

	fs.BoolVar(
		&c.clearOnReset,
		"clear-on-reset",
		false,
		"clear all data from storage when the application resets",
	)
}

// exec executes the indexer start command
func (c *startCfg) exec(ctx context.Context) error {
	// Parse the log level
	logLevel, err := zap.ParseAtomicLevel(c.logLevel)
	if err != nil {
		return fmt.Errorf("unable to parse log level, %w", err)
	}

	cfg := zap.NewDevelopmentConfig()
	cfg.Level = logLevel

	// Create a new logger
	logger, err := cfg.Build()
	if err != nil {
		return fmt.Errorf("unable to create logger, %w", err)
	}

	// Create a DB instance
	db, err := storage.NewPebble(c.dbPath)
	if err != nil {
		return fmt.Errorf("unable to open storage DB, %w", err)
	}

	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			logger.Error("unable to gracefully close DB", zap.Error(closeErr))
		}
	}()

	// Create an Event Manager instance
	em := events.NewManager()

	// Create a TM2 client
	tm2Client, err := client.NewClient(c.remote)
	if err != nil {
		return fmt.Errorf("unable to create client, %w", err)
	}

	// Create the fetcher service
	f := fetch.New(
		db,
		tm2Client,
		em,
		fetch.WithLogger(
			logger.Named("fetcher"),
		),
		fetch.WithMaxSlots(c.maxSlots),
		fetch.WithMaxChunkSize(c.maxChunkSize),
		fetch.WithClearOnReset(c.clearOnReset),
		fetch.WithDBPath(c.dbPath),
	)

	// The supply handler serves both the JSON-RPC and GraphQL surfaces from
	// one shared snapshot. It only ever queries the tracked denoms, on its
	// own schedule, so request input never reaches the chain.
	denoms, err := parseSupplyDenoms(c.supplyDenoms)
	if err != nil {
		return err
	}

	supplyHandler := supply.NewHandler(
		tm2Client,
		db,
		supply.WithLogger(
			logger.Named("supply"),
		),
		supply.WithDenoms(denoms),
	)

	// Create the JSON-RPC service
	j := setupJSONRPC(
		db,
		supplyHandler,
		em,
		logger,
	)

	mux := chi.NewMux()

	mux.Use(cors.Handler(cors.Options{
		AllowedOrigins:   strings.Split(c.corsAllowedOrigins, ","),
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: false,
	}))

	if c.rateLimit != 0 {
		logger.Info("rate-limit set", zap.Int("rate-limit", c.rateLimit))
		mux.Use(httprate.Limit(
			c.rateLimit,
			1*time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByRealIP),
			httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
				//nolint:errcheck // no need to handle error here, it had been checked before
				ip, _ := httprate.KeyByRealIP(r)
				logger.Debug("too many requests", zap.String("from", ip))

				// send a json response to give more info when using the graphQL explorer
				http.Error(w, `{"error": "too many requests"}`, http.StatusTooManyRequests)
			}),
		))
	}

	mux = j.SetupRoutes(mux)
	mux = graph.Setup(db, em, supplyHandler, mux, c.disableIntrospection)
	mux = health.Setup(db, f, mux)

	// Create the HTTP server
	hs := serve.NewHTTPServer(mux, c.listenAddress, logger.Named("http-server"))

	// Create a new waiter
	w := newWaiter(ctx)

	// genesisReady is closed once the chain genesis is in the storage. Two
	// services read it from there and so wait on it: the fetcher indexes from
	// height 0, and the supply handler folds the genesis balances into vesting
	// schedules.
	genesisReady := make(chan struct{})

	// Add the genesis bootstrap. It runs beside the HTTP server rather than
	// ahead of it, because it retries until the node answers: a restart during
	// a node outage keeps serving what the storage already holds instead of
	// leaving every port shut until the node is back. A no-op once the storage
	// carries this chain's genesis.
	w.add(func(ctx context.Context) error {
		if err := genesis.Bootstrap(
			ctx,
			db,
			tm2Client,
			genesis.WithLogger(
				logger.Named("genesis"),
			),
		); err != nil {
			if errors.Is(err, context.Canceled) {
				// Shut down while still retrying, like the other services
				// returning on a cancelled context
				return nil
			}

			return fmt.Errorf("unable to bootstrap genesis, %w", err)
		}

		close(genesisReady)

		return nil
	})

	// Add the JSON-RPC service
	w.add(hs.Serve)

	// Add the fetcher service
	w.add(afterGenesis(genesisReady, f.FetchChainData))

	// Add the supply snapshot refresher
	w.add(afterGenesis(genesisReady, supplyHandler.Start))

	// Wait for the services to stop
	return errors.Join(
		w.wait(),
		logger.Sync(),
	)
}

// afterGenesis holds a service back until the genesis bootstrap has stored the
// chain genesis, for the services that read it from the storage. A shutdown
// before the bootstrap succeeds never starts the service at all.
func afterGenesis(ready <-chan struct{}, fn waitFunc) waitFunc {
	return func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return nil
		case <-ready:
			return fn(ctx)
		}
	}
}

// parseSupplyDenoms splits the comma-separated flag value, validates each
// denomination, and refuses an empty list. A typo or an empty value should
// fail at startup instead of leaving getSupply to answer nothing but
// errors.
func parseSupplyDenoms(raw string) ([]string, error) {
	var denoms []string

	for _, denom := range strings.Split(raw, ",") {
		denom = strings.TrimSpace(denom)
		if denom == "" {
			continue
		}

		if err := std.ValidateDenom(denom); err != nil {
			return nil, fmt.Errorf("invalid supply denom %q: %w", denom, err)
		}

		if !slices.Contains(denoms, denom) {
			denoms = append(denoms, denom)
		}
	}

	if len(denoms) == 0 {
		return nil, errors.New("no supply denoms configured: --supply-denoms must list at least one denomination")
	}

	return denoms, nil
}

// setupJSONRPC sets up the JSONRPC instance
func setupJSONRPC(
	db *storage.Pebble,
	supplyHandler *supply.Handler,
	em *events.Manager,
	logger *zap.Logger,
) *serve.JSONRPC {
	j := serve.NewJSONRPC(
		em,
		serve.WithLogger(
			logger.Named("json-rpc"),
		),
	)

	// Transaction handlers
	j.RegisterTxEndpoints(db)

	// Gas handlers
	j.RegisterGasEndpoints(db)

	// Block handlers
	j.RegisterBlockEndpoints(db)

	// Sub handlers
	j.RegisterSubEndpoints(db)

	// Supply handlers
	j.RegisterSupplyEndpoints(supplyHandler)

	return j
}
