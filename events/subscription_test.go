package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSubscription_EventSupported(t *testing.T) {
	t.Parallel()

	supportedEvents := []Type{
		"dummy 1",
		"dummy 2",
	}

	subscription := &eventSubscription{
		eventTypes: supportedEvents,
	}

	testTable := []struct {
		name      string
		events    []Type
		supported bool
	}{
		{
			"Supported events processed",
			supportedEvents,
			true,
		},
		{
			"Unsupported events not processed",
			[]Type{
				"random event 1",
				"random event 2",
			},
			false,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, eventType := range testCase.events {
				assert.Equal(t, testCase.supported, subscription.eventSupported(eventType))
			}
		})
	}
}

// retryUntilTimeout retries the callback until it returns false,
// otherwise it times out when the context is cancelled
func retryUntilTimeout(ctx context.Context, t *testing.T, cb func() bool) error {
	t.Helper()

	resCh := make(chan error, 1)

	go func() {
		defer close(resCh)

		for {
			select {
			case <-ctx.Done():
				resCh <- errors.New("timeout")

				return
			default:
				retry := cb()

				if !retry {
					resCh <- nil

					return
				}
			}

			time.Sleep(time.Millisecond * 100)
		}
	}()

	return <-resCh
}
