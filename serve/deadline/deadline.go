// Package deadline bounds how long one GraphQL or JSON-RPC query may run, so
// one client's long scans cannot hold the indexer's CPU.
package deadline

import (
	"context"
	"fmt"
	"time"
)

// ExceededError is the cause of a query's context once the query deadline has
// passed. Its message names the deadline, so a client can tell a query the
// server stopped from one that failed.
type ExceededError struct {
	// Deadline is the configured query deadline
	Deadline time.Duration
}

func (e *ExceededError) Error() string {
	return fmt.Sprintf("query deadline exceeded (%s); narrow the block height range", e.Deadline)
}

// Details are the error's machine-readable fields, carried beside the message
// in the GraphQL error's extensions and the JSON-RPC error's data. Clients
// read them rather than the message text.
func (e *ExceededError) Details() map[string]any {
	return map[string]any{
		"code":          "QUERY_DEADLINE_EXCEEDED",
		"queryDeadline": e.Deadline.String(),
	}
}

// WithTimeout bounds ctx by the query deadline d. Once d has passed, ctx is
// done and context.Cause(ctx) is an *ExceededError. A d of zero or less
// leaves ctx unbounded, which is how the deadline is turned off.
func WithTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return ctx, func() {}
	}

	return context.WithTimeoutCause(ctx, d, &ExceededError{Deadline: d})
}
