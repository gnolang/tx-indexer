package serve

import "go.uber.org/zap"

type Option func(s *JSONRPC)

// WithLogger sets the logger to be used
// with the JSON-RPC server
func WithLogger(logger *zap.Logger) Option {
	return func(s *JSONRPC) {
		s.logger = logger
	}
}

// WithListenAddress sets the listen address
// for the JSON-RPC server
func WithListenAddress(address string) Option {
	return func(s *JSONRPC) {
		s.listenAddress = address
	}
}
