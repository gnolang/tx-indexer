package fetch

import "go.uber.org/zap"

type Option func(f *Fetcher)

// WithLogger sets the logger to be used
// with the fetcher
func WithLogger(logger *zap.Logger) Option {
	return func(f *Fetcher) {
		f.logger = logger
	}
}
