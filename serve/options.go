package serve

import (
	"time"

	"go.uber.org/zap"
)

type Option func(s *JSONRPC)

// WithLogger sets the logger to be used
// with the JSON-RPC server
func WithLogger(logger *zap.Logger) Option {
	return func(s *JSONRPC) {
		s.logger = logger
	}
}

// WithQueryDeadline sets how long one request may run before its calls
// answer a deadline error. Zero, the default, leaves requests unbounded.
func WithQueryDeadline(d time.Duration) Option {
	return func(s *JSONRPC) {
		s.queryDeadline = d
	}
}
