package serve

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	DefaultListenAddress = "0.0.0.0:8545"
)

type HTTPServer struct {
	h      http.Handler
	logger *zap.Logger
	addr   string
}

func NewHTTPServer(h http.Handler, addr string, logger *zap.Logger) *HTTPServer {
	return &HTTPServer{h: h, addr: addr, logger: logger}
}

// Serve serves the JSON-RPC server
func (s *HTTPServer) Serve(ctx context.Context) error {
	faucet := &http.Server{
		Addr:              s.addr,
		Handler:           s.h,
		ReadHeaderTimeout: 60 * time.Second,
	}

	group, gCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		defer s.logger.Info("HTTP server shut down")

		ln, err := net.Listen("tcp", faucet.Addr)
		if err != nil {
			return err
		}

		s.logger.Info(
			"HTTP server started",
			zap.String("address", ln.Addr().String()),
		)

		if err := faucet.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}

		return nil
	})

	group.Go(func() error {
		<-gCtx.Done()

		s.logger.Info("HTTP server to be shut down")

		wsCtx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()

		return faucet.Shutdown(wsCtx)
	})

	return group.Wait()
}
