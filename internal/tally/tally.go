// Package tally counts the rows one served request reads from the storage and
// the rows it answers. It depends on the standard library alone, so the
// storage counts the keys it walks without importing what the counts feed.
package tally

import (
	"context"
	"sync/atomic"
)

// Rows counts the rows one served request reads from the storage and the rows
// it answers. The request carries it in its context, so the storage counts the
// keys its iterators walk without knowing what is served. A nil Rows counts
// nothing.
type Rows struct {
	scanned  atomic.Int64
	returned atomic.Int64
}

// contextKey is the context key a Rows is carried under
type contextKey struct{}

// NewContext returns a copy of ctx that carries rows
func NewContext(ctx context.Context, rows *Rows) context.Context {
	return context.WithValue(ctx, contextKey{}, rows)
}

// FromContext returns the Rows ctx carries, or nil if it carries none
func FromContext(ctx context.Context) *Rows {
	rows, _ := ctx.Value(contextKey{}).(*Rows)

	return rows
}

// AddScanned counts n rows read from the storage, the ones a filter or an
// index window then drops included
func (r *Rows) AddScanned(n int64) {
	if r == nil {
		return
	}

	r.scanned.Add(n)
}

// AddReturned counts n rows the request answers
func (r *Rows) AddReturned(n int64) {
	if r == nil {
		return
	}

	r.returned.Add(n)
}

// Scanned is the number of rows read from the storage so far
func (r *Rows) Scanned() int64 {
	if r == nil {
		return 0
	}

	return r.scanned.Load()
}

// Returned is the number of rows answered so far
func (r *Rows) Returned() int64 {
	if r == nil {
		return 0
	}

	return r.returned.Load()
}
