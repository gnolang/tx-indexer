package subscription

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test that makes the coverage gods happy
func TestBaseSubscription_WriteResponse(t *testing.T) {
	t.Parallel()

	// Create base subscription
	s := newBaseSubscription(nil)

	assert.Nil(t, s.WriteResponse(nil))
}
