package genesis

import (
	"time"

	"go.uber.org/zap"
)

type Option func(c *config)

type config struct {
	logger  *zap.Logger
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
