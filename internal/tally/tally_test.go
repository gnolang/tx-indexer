package tally

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The Rows a context carries collect every row counted against that context,
// the storage's walk and the handler's answer alike
func TestRows_CountsAgainstItsContext(t *testing.T) {
	t.Parallel()

	rows := &Rows{}
	ctx := NewContext(context.Background(), rows)

	FromContext(ctx).AddScanned(3)
	FromContext(ctx).AddScanned(2)
	FromContext(ctx).AddReturned(1)

	assert.Equal(t, int64(5), rows.Scanned())
	assert.Equal(t, int64(1), rows.Returned())
}

// A context that carries no Rows counts nothing, so the code that counts rows
// runs the same outside a served request
func TestRows_NoneInTheContextCountsNothing(t *testing.T) {
	t.Parallel()

	rows := FromContext(context.Background())
	assert.Nil(t, rows)

	assert.NotPanics(t, func() {
		rows.AddScanned(1)
		rows.AddReturned(1)
	})

	assert.Zero(t, rows.Scanned())
	assert.Zero(t, rows.Returned())
}
