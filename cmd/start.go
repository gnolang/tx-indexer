package main

import (
	"context"
	"flag"

	"github.com/peterbourgon/ff/v3/ffcli"
)

type startCfg struct{}

// newStartCmd creates the indexer start command
func newStartCmd() *ffcli.Command {
	cfg := &startCfg{}

	fs := flag.NewFlagSet("start", flag.ExitOnError)
	cfg.registerFlags(fs)

	return &ffcli.Command{
		Name:       "start",
		ShortUsage: "start [flags]",
		LongHelp:   "Starts the transaction indexer",
		FlagSet:    fs,
		Exec:       cfg.exec,
	}
}

// registerFlags registers the indexer start command flags
func (c *startCfg) registerFlags(_ *flag.FlagSet) {
	// TODO define flags
}

// exec executes the indexer start command
func (c *startCfg) exec(_ context.Context, _ []string) error {
	// TODO add implementation
	return nil
}
