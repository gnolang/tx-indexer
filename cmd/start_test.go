package main

import (
	"context"
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAfterGenesis_WaitsForBootstrap makes sure a service that reads the
// genesis from the storage only starts once the bootstrap has put it there.
func TestAfterGenesis_WaitsForBootstrap(t *testing.T) {
	t.Parallel()

	var (
		ready   = make(chan struct{})
		started = make(chan struct{})
	)

	go func() {
		require.NoError(t, afterGenesis(ready, func(context.Context) error {
			close(started)

			return nil
		})(t.Context()))
	}()

	select {
	case <-started:
		require.FailNow(t, "the service started before the genesis was bootstrapped")
	case <-time.After(50 * time.Millisecond):
	}

	close(ready)

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the service never started")
	}
}

// TestAfterGenesis_ShutdownBeforeBootstrap makes sure a shutdown while the
// bootstrap is still retrying never starts the service, and stops cleanly.
func TestAfterGenesis_ShutdownBeforeBootstrap(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.NoError(t, afterGenesis(make(chan struct{}), func(context.Context) error {
		require.FailNow(t, "the service must not start")

		return nil
	})(ctx))
}

// TestStartCfg_QueryDeadlineFlag makes sure the query deadline is off unless
// the operator sets it, and takes a Go duration.
func TestStartCfg_QueryDeadlineFlag(t *testing.T) {
	t.Parallel()

	unset := &startCfg{}
	unsetFlags := flag.NewFlagSet("start", flag.ContinueOnError)
	unset.registerFlags(unsetFlags)

	require.NoError(t, unsetFlags.Parse(nil))
	assert.Zero(t, unset.queryDeadline)

	set := &startCfg{}
	setFlags := flag.NewFlagSet("start", flag.ContinueOnError)
	set.registerFlags(setFlags)

	require.NoError(t, setFlags.Parse([]string{"--query-deadline", "10s"}))
	assert.Equal(t, 10*time.Second, set.queryDeadline)
}

// TestStartCfg_NegativeQueryDeadline makes sure a negative deadline fails at
// startup rather than run with the deadline silently off.
func TestStartCfg_NegativeQueryDeadline(t *testing.T) {
	t.Parallel()

	cfg := &startCfg{
		queryDeadline: -time.Second,
		dbPath:        t.TempDir(),
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorContains(t, cfg.exec(ctx), "--query-deadline")
}
