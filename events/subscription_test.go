package events

import (
	"testing"

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
