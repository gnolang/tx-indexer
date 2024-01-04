package filters

import (
	"time"
)

type Option func(*Manager)

// WithCleanupInterval creates a filter manager with the specified
// cleanup interval
func WithCleanupInterval(interval time.Duration) Option {
	return func(manager *Manager) {
		manager.cleanupInterval = interval
	}
}
