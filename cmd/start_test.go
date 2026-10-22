package main

import (
	"context"
	"testing"
	"time"

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
