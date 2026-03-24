package genesis

import (
	"time"

	"go.uber.org/zap"
)

type Option func(c *config)

type config struct {
	logger     *zap.Logger
	backoff    time.Duration
	genesisURL string
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

// WithURL sets the fallback URL to download genesis.json when the RPC call fails.
func WithURL(url string) Option {
	return func(c *config) {
		c.genesisURL = url
	}
}
