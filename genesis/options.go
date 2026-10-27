package genesis

import (
	"time"

	"go.uber.org/zap"

	"github.com/gnolang/tx-indexer/metrics"
)

type Option func(c *config)

type config struct {
	logger  *zap.Logger
	metrics *metrics.Metrics
	backoff time.Duration
}

// WithLogger sets the logger to be used with the bootstrap
func WithLogger(logger *zap.Logger) Option {
	return func(c *config) {
		c.logger = logger
	}
}

// WithBackoff sets the pause between bootstrap attempts
func WithBackoff(backoff time.Duration) Option {
	return func(c *config) {
		c.backoff = backoff
	}
}

// WithMetrics sets the metrics the bootstrap reports its failed reads of the
// chain's status to. Unset, it reports none.
func WithMetrics(m *metrics.Metrics) Option {
	return func(c *config) {
		c.metrics = m
	}
}
