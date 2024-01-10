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
	}
}
