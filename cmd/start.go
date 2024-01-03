package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/gnolang/tx-indexer/client"
	"github.com/gnolang/tx-indexer/fetch"
	"github.com/gnolang/tx-indexer/serve"
	"github.com/gnolang/tx-indexer/serve/handlers/block"
	"github.com/gnolang/tx-indexer/serve/handlers/tx"
	"github.com/gnolang/tx-indexer/storage"
	"github.com/peterbourgon/ff/v3/ffcli"
	"go.uber.org/zap"
)

const (
	defaultRemote = "http://127.0.0.1:26657"
	defaultDBPath = "indexer-db"
)

type startCfg struct {
	listenAddress string
	remote        string
	dbPath        string
	logLevel      string
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
	defer logger.Sync()

	// Create a DB instance
	db, err := storage.New(c.dbPath)
	if err != nil {
		return fmt.Errorf("unable to open storage DB, %w", err)
	}

	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("unable to gracefully close DB", zap.Error(err))
		}
	}()

	// Create the fetcher instance
	f := fetch.New(
		db,
		client.NewClient(c.remote),
		fetch.WithLogger(
			logger.Named("fetcher"),
		),
	)

	// Create the JSON-RPC service
	j := serve.NewJSONRPC(
		serve.WithLogger(
			logger.Named("json-rpc"),
		),
		serve.WithListenAddress(
			c.listenAddress,
		),
	)

	txHandler := tx.NewHandler(db)
	blockHandler := block.NewHandler(db)

	// Register handlers
	j.RegisterHandler(
		"getTx",
		txHandler.GetTxHandler,
	)

	j.RegisterHandler(
		"getBlock",
		blockHandler.GetBlockHandler,
	)

	// Create a new waiter
	w := newWaiter(ctx)

	// Add the fetcher service
	w.add(f.FetchTransactions)

	// Add the JSON-RPC service
	w.add(j.Serve)

	// Wait for the services to stop
	return w.wait()
}
