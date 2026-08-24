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

	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/gnolang/tx-indexer/client"
	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/fetch"
	"github.com/gnolang/tx-indexer/serve"
	"github.com/gnolang/tx-indexer/serve/graph"
	"github.com/gnolang/tx-indexer/serve/handlers/supply"
	"github.com/gnolang/tx-indexer/serve/health"
	"github.com/gnolang/tx-indexer/storage"
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
	)

	// Bootstrap the chain genesis before any service starts: the fetcher
	// needs the genesis block stored, and the supply handler needs the
	// genesis balances where the vesting schedules live. One fetch, one
	// decode, both consumers.
	genesisBalances, err := f.BootstrapGenesis(ctx)
	if err != nil {
		return fmt.Errorf("unable to bootstrap genesis, %w", err)
	}

	// The supply handler serves both the JSON-RPC and GraphQL surfaces, so
	// they share one snapshot. Only the tracked denoms are ever queried, on
	// the handler's own schedule — request input cannot reach the chain.
	denoms, err := parseSupplyDenoms(c.supplyDenoms)
	if err != nil {
		return err
	}

	vestings, err := supply.NewVestings(genesisBalances)
	if err != nil {
		return fmt.Errorf("unable to parse genesis vesting schedules, %w", err)
	}

	supplyHandler := supply.NewHandler(
		tm2Client,
		supply.WithLogger(
			logger.Named("supply"),
		),
		supply.WithDenoms(denoms),
		supply.WithVestings(vestings),
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

	// Add the fetcher service
	w.add(f.FetchChainData)

	// Add the supply snapshot refresher
	w.add(supplyHandler.Start)

	// Add the JSON-RPC service
	w.add(hs.Serve)

	// Wait for the services to stop
	return errors.Join(
		w.wait(),
		logger.Sync(),
	)
}

// parseSupplyDenoms splits the comma-separated flag value, validates each
// denomination and refuses an empty list — an operator's typo or an empty
// value fails at startup rather than leaving getSupply answering nothing
// but errors.
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
