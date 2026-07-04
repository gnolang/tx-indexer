package fetch

import (
	"time"

	"go.uber.org/zap"
)

type Option func(f *Fetcher)

// WithLogger sets the logger to be used
// with the fetcher
func WithLogger(logger *zap.Logger) Option {
	return func(f *Fetcher) {
		f.logger = logger
	}
}

// WithMaxSlots sets the maximum worker slots
// for the fetcher
func WithMaxSlots(maxSlots int) Option {
	return func(f *Fetcher) {
		f.maxSlots = maxSlots
	}
}

// WithMaxChunkSize sets the maximum worker
// chunk size (data range) for the fetcher
func WithMaxChunkSize(maxChunkSize int64) Option {
	return func(f *Fetcher) {
		f.maxChunkSize = maxChunkSize
		f.latestChunkSize = int(maxChunkSize)
	}
}

// WithClearOnReset sets the clear on reset flag
func WithClearOnReset(clearOnReset bool) Option {
	return func(f *Fetcher) {
		f.clearOnReset = clearOnReset
	}
}

// WithDBPath sets the database path
func WithDBPath(dbPath string) Option {
	return func(f *Fetcher) {
		f.dbPath = dbPath
	}
}

// WithChunkFetchRetry configures how failed block / tx-result fetches are retried.
// Only the heights that fail are retried; already-fetched blocks are kept.
// maxRetries is the number of retry rounds, each preceded by delay.
func WithChunkFetchRetry(maxRetries int, delay time.Duration) Option {
	return func(f *Fetcher) {
		f.retry = retryConfig{
			maxRetries: maxRetries,
			retryDelay: delay,
		}
	}
}

// WithBackfillInterval sets how often the backfiller drains queued gaps.
// A non-positive interval disables the backfiller entirely.
func WithBackfillInterval(interval time.Duration) Option {
	return func(f *Fetcher) {
		f.backfillInterval = interval
	}
}

// WithStartupAudit controls whether the backfiller scans existing storage for
// missing-block gaps on startup.
func WithStartupAudit(enabled bool) Option {
	return func(f *Fetcher) {
		f.auditOnStart = enabled
	}
}

// WithTxAudit controls whether the startup audit also queues blocks
// that are stored but missing some or all of their transactions.
// It must decode every block to read the declared tx count,
// so it is much more expensive than the plain missing-block audit and is opt-in.
func WithTxAudit(enabled bool) Option {
	return func(f *Fetcher) {
		f.txAudit = enabled
	}
}

// WithAuditFromHeight limits both startup audits to heights >= from, skipping
// everything below it. Use it to scan only the range affected by an incident
// instead of the whole history.
func WithAuditFromHeight(from uint64) Option {
	return func(f *Fetcher) {
		f.auditFromHeight = from
	}
}

// WithTxAuditThrottle tunes the tx-completeness audit for constrained
// deployments: it processes windowBlocks heights, then pauses for nap before
// the next window. A smaller window and larger nap lower the audit's CPU share
// (at the cost of a longer scan). The window is also the resume granularity:
// progress is persisted per completed window so restarts continue instead of
// re-scanning from the start.
func WithTxAuditThrottle(windowBlocks int, nap time.Duration) Option {
	return func(f *Fetcher) {
		f.txAuditWindow = windowBlocks
		f.txAuditNap = nap
	}
}
