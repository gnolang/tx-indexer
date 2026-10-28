package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/peterbourgon/ff/v3/ffcli"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/gnolang/tx-indexer/client"
	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/fetch"
	"github.com/gnolang/tx-indexer/genesis"
	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve"
	"github.com/gnolang/tx-indexer/serve/graph"
	"github.com/gnolang/tx-indexer/serve/handlers/supply"
	"github.com/gnolang/tx-indexer/serve/health"
	"github.com/gnolang/tx-indexer/serve/observe"
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

// queryDeadlineHelp is built up over multiple lines so each stays under the
// linter's line-length limit.
const queryDeadlineHelp = "the maximum time a GraphQL or JSON-RPC request may run before it is answered " +
	"with a deadline error, unlimited by default; subscriptions are exempt"

// metricsListenAddressHelp is built up over multiple lines so each stays
// under the linter's line-length limit.
const metricsListenAddressHelp = "the IP:PORT address to serve the Prometheus metrics on, at /metrics, " +
	"apart from the public endpoints; off by default"

// requestLogHelp is built up over multiple lines so each stays under the
// linter's line-length limit.
const requestLogHelp = "log one line per GraphQL root field or JSON-RPC call served, at info: its operation, " +
	"status, duration, rows and client address, never its arguments"

// clientIPHeaderHelp is built up over multiple lines so each stays under the
// linter's line-length limit.
const clientIPHeaderHelp = "the header trusted to carry the client address the request log names, " +
	"such as CF-Connecting-IP or X-Forwarded-For; unset, the address the request came from"

// clientIPTrustedHopsHelp is built up over multiple lines so each stays under
// the linter's line-length limit.
const clientIPTrustedHopsHelp = "the number of trusted proxies appending to --client-ip-header: " +
	"the client address is that many entries from the right of its list"

// The formats the log lines are written in, the values of --log-format
const (
	logFormatConsole = "console"
	logFormatJSON    = "json"
)

// logFormatHelp is built up over multiple lines so each stays under the
// linter's line-length limit.
const logFormatHelp = "the format of the log lines: console, columns for people to read, " +
	"or json, one object per line for log pipelines to parse"

type startCfg struct {
	listenAddress        string
	metricsListenAddress string
	remote               string
	dbPath               string
	logLevel             string
	logFormat            string
	corsAllowedOrigins   string
	supplyDenoms         string
	clientIPHeader       string
	maxSlots             int
	maxChunkSize         int64
	rateLimit            int
	clientIPTrustedHops  int
	queryDeadline        time.Duration
	disableIntrospection bool
	requestLog           bool
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
		&c.metricsListenAddress,
		"metrics-listen-address",
		"",
		metricsListenAddressHelp,
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

	fs.StringVar(
		&c.logFormat,
		"log-format",
		logFormatConsole,
		logFormatHelp,
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

	fs.DurationVar(
		&c.queryDeadline,
		"query-deadline",
		0,
		queryDeadlineHelp,
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
		&c.requestLog,
		"request-log",
		false,
		requestLogHelp,
	)

	fs.StringVar(
		&c.clientIPHeader,
		"client-ip-header",
		"",
		clientIPHeaderHelp,
	)

	fs.IntVar(
		&c.clientIPTrustedHops,
		"client-ip-trusted-hops",
		1,
		clientIPTrustedHopsHelp,
	)
}

// exec executes the indexer start command
func (c *startCfg) exec(ctx context.Context) error {
	// A negative deadline is a typo rather than a setting: refuse it instead
	// of serving with the deadline off
	if c.queryDeadline < 0 {
		return fmt.Errorf("invalid --query-deadline %s: it must be 0 (off) or positive", c.queryDeadline)
	}

	// No trusted proxy would leave every entry of the header the client's own
	if c.clientIPTrustedHops < 1 {
		return fmt.Errorf("invalid --client-ip-trusted-hops %d: it must be 1 or more", c.clientIPTrustedHops)
	}

	// Parse the log level
	logLevel, err := zap.ParseAtomicLevel(c.logLevel)
	if err != nil {
		return fmt.Errorf("unable to parse log level, %w", err)
	}

	// Create a new logger
	logger, err := newLogger(c.logFormat, logLevel)
	if err != nil {
		return fmt.Errorf("unable to create logger, %w", err)
	}

	// A dependency's line through the standard log package, as gqlgen writes
	// a failed WebSocket upgrade, is a line of the logger, at warn: no format
	// has a plain line any client can cause. It carries a stack only from
	// error up, as a stack per request would fill the log.
	stdLogger := logger.Named("stdlib").WithOptions(zap.AddStacktrace(zap.ErrorLevel))

	restoreStdLog, err := zap.RedirectStdLogAt(stdLogger, zap.WarnLevel)
	if err != nil {
		return fmt.Errorf("unable to redirect the standard logger, %w", err)
	}

	defer restoreStdLog()

	// The command writes its own lines under the start logger, as each service
	// writes under its own: every line names the logger it came from
	startLogger := logger.Named("start")

	return stopped(c.run(ctx, logger, startLogger), logger, startLogger, os.Stderr)
}

// stopped is the error a run ends on, once it has returned runErr: runErr
// is logged at error under startLogger, then logger is synced to out, its
// output. An error the log holds is a loggedError, which main does not print
// again; one the level leaves out of the log is returned as it is, for main
// to print. A failed sync, which the logger cannot log, is returned with the
// run's error when the log does not hold that, and alone when it does.
func stopped(runErr error, logger, startLogger *zap.Logger, out *os.File) error {
	if runErr != nil {
		if entry := startLogger.Check(zap.ErrorLevel, "stopped on an error"); entry != nil {
			entry.Write(zap.Error(runErr))

			runErr = &loggedError{err: runErr}
		}
	}

	syncErr := syncLogger(logger, out)
	if syncErr == nil {
		return runErr
	}

	var logged *loggedError
	if errors.As(runErr, &logged) {
		return syncErr
	}

	return errors.Join(runErr, syncErr)
}

// loggedError is an error the start command has logged already, so main does
// not print it again
type loggedError struct {
	err error
}

func (e *loggedError) Error() string {
	return e.err.Error()
}

// Unwrap is the error that was logged
func (e *loggedError) Unwrap() error {
	return e.err
}

// run runs the indexer's services until ctx ends or one of them fails,
// writing through logger, the command's own lines through startLogger
func (c *startCfg) run(ctx context.Context, logger, startLogger *zap.Logger) error {
	// Create a DB instance
	db, err := storage.NewPebble(c.dbPath, logger.Named("pebble"))
	if err != nil {
		return fmt.Errorf("unable to open storage DB, %w", err)
	}

	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			startLogger.Error("unable to gracefully close DB", zap.Error(closeErr))
		}
	}()

	// Create an Event Manager instance
	em := events.NewManager()

	// Create a TM2 client
	tm2Client, err := client.NewClient(c.remote)
	if err != nil {
		return fmt.Errorf("unable to create client, %w", err)
	}

	// The metrics are collected only when they are served
	var m *metrics.Metrics

	if c.metricsListenAddress != "" {
		m = metrics.New()
	}

	observer := c.newObserver(m, logger)

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
		fetch.WithMetrics(m),
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
		c.queryDeadline,
		observer,
	)

	mux := chi.NewMux()

	mux.Use(cors.Handler(cors.Options{
		AllowedOrigins:   strings.Split(c.corsAllowedOrigins, ","),
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: false,
	}))

	if c.rateLimit != 0 {
		startLogger.Info("rate-limit set", zap.Int("rate-limit", c.rateLimit))
		mux.Use(httprate.Limit(
			c.rateLimit,
			1*time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByRealIP),
			httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
				//nolint:errcheck // no need to handle error here, it had been checked before
				ip, _ := httprate.KeyByRealIP(r)
				startLogger.Debug("too many requests", zap.String("from", ip))

				// send a json response to give more info when using the graphQL explorer
				http.Error(w, `{"error": "too many requests"}`, http.StatusTooManyRequests)
			}),
		))
	}

	if c.queryDeadline != 0 {
		startLogger.Info("query deadline set", zap.Duration("query-deadline", c.queryDeadline))
	}

	if c.requestLog {
		startLogger.Info(
			"request log set",
			zap.String("client-ip-header", c.clientIPHeader),
			zap.Int("client-ip-trusted-hops", c.clientIPTrustedHops),
		)
	}

	mux = j.SetupRoutes(mux)
	mux = graph.Setup(db, em, supplyHandler, mux, graph.Options{
		Observer:             observer,
		Logger:               logger.Named("graphql"),
		QueryDeadline:        c.queryDeadline,
		DisableIntrospection: c.disableIntrospection,
	})
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
			genesis.WithMetrics(m),
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

	// Add the metrics server. It listens apart from the public endpoints, so
	// the operator decides who reaches it
	if m != nil {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("GET /metrics", m.Handler())

		w.add(serve.NewHTTPServer(metricsMux, c.metricsListenAddress, logger.Named("metrics-server")).Serve)
	}

	// Add the fetcher service
	w.add(afterGenesis(genesisReady, f.FetchChainData))

	// Add the supply snapshot refresher
	w.add(afterGenesis(genesisReady, supplyHandler.Start))

	// Wait for the services to stop
	return w.wait()
}

// syncLogger flushes logger to storage when out, the stderr it writes to, is a
// regular file: zap writes each line to out as it logs it, and only a file
// holds lines the kernel has yet to store. A terminal, a pipe or a socket, as
// Docker and systemd hand the indexer, takes no sync: zap's Sync fails on one
// with an error that differs by OS and by kind of file (EINVAL on Linux;
// EBADF, ENODEV or ENOTTY on macOS: uber-go/zap#370 and #880), and would end
// a clean stop with exit 1.
func syncLogger(logger *zap.Logger, out *os.File) error {
	info, err := out.Stat()
	if err != nil {
		return fmt.Errorf("unable to stat the log output, %w", err)
	}

	if !info.Mode().IsRegular() {
		return nil
	}

	return logger.Sync()
}

// newLogger builds the logger the command writes with, at level, in format.
// It is zap's development config, its console encoder for the console
// format. Its output paths are stderr alone, in either format, and syncLogger
// judges os.Stderr at the stop on that ground: an output path set here goes
// unsynced unless syncLogger is given it too.
func newLogger(format string, level zap.AtomicLevel) (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	cfg.Level = level

	switch format {
	case logFormatConsole:
		// The development config encodes it
	case logFormatJSON:
		cfg.Encoding = "json"
		cfg.EncoderConfig = jsonEncoderConfig()
	default:
		return nil, fmt.Errorf("invalid --log-format %q: it must be %s or %s", format, logFormatConsole, logFormatJSON)
	}

	return cfg.Build()
}

// jsonEncoderConfig encodes each line as one JSON object, its fields at the
// top level beside ts, level, logger, caller and msg, and a stack trace under
// stacktrace. It is zap's production encoder config, but for durations, as
// text, and the time, in RFC 3339 with nanoseconds, in UTC.
func jsonEncoderConfig() zapcore.EncoderConfig {
	encoder := zap.NewProductionEncoderConfig()

	encoder.EncodeTime = encodeUTCTime
	encoder.EncodeDuration = zapcore.StringDurationEncoder

	return encoder
}

// utcTimeLayout is RFC 3339 with all nine digits of nanoseconds, so every
// line's time has one width, a whole second included
const utcTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// encodeUTCTime writes t in RFC 3339 with nanoseconds, in UTC
func encodeUTCTime(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.UTC().Format(utcTimeLayout))
}

// newObserver creates the observer of the served requests: it records them
// into m, and logs each under logger when the request log is on. With
// neither, it is nil, and serving a request costs nothing more.
func (c *startCfg) newObserver(m *metrics.Metrics, logger *zap.Logger) *observe.Observer {
	if m == nil && !c.requestLog {
		return nil
	}

	var opts []observe.Option

	if c.requestLog {
		opts = append(
			opts,
			observe.WithRequestLog(logger.Named("requests")),
			observe.WithClientIPHeader(c.clientIPHeader, c.clientIPTrustedHops),
		)
	}

	return observe.New(m, opts...)
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
	queryDeadline time.Duration,
	observer *observe.Observer,
) *serve.JSONRPC {
	j := serve.NewJSONRPC(
		em,
		serve.WithLogger(
			logger.Named("json-rpc"),
		),
		serve.WithQueryDeadline(queryDeadline),
		serve.WithObserver(observer),
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
