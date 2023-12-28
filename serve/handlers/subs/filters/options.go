package filters

import (
	"time"

	"go.uber.org/zap"
)

type Option func(*Manager)

// WithLogger creates a filter manager with the specified Zap logger
func WithLogger(logger *zap.Logger) Option {
	return func(manager *Manager) {
		manager.logger = logger
	}
}

// WithCleanupInterval creates a filter manager with the specified
// cleanup interval
func WithCleanupInterval(interval time.Duration) Option {
	return func(manager *Manager) {
		manager.cleanupInterval = interval
	}
}
