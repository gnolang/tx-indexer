package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/peterbourgon/ff/v3/ffcli"
)

func main() {
	fs := flag.NewFlagSet("root", flag.ExitOnError)

	// Create the root command
	cmd := &ffcli.Command{
		ShortUsage: "<subcommand> [flags] [<arg>...]",
		LongHelp:   "The Gno / TM2 faucet service",
		FlagSet:    fs,
		Exec: func(_ context.Context, _ []string) error {
			return flag.ErrHelp
		},
	}

	// Add the subcommands
	cmd.Subcommands = []*ffcli.Command{
		newStartCmd(),
		// newResetCmd(),
		// newRepairCmd(),
	}

	if err := cmd.ParseAndRun(context.Background(), os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%+v", err)

		os.Exit(1)
	}
}
