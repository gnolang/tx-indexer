package http

import (
	"encoding/json"
	"net/http"

	"github.com/gnolang/tx-indexer/serve/writer"
	"go.uber.org/zap"
)

var _ writer.ResponseWriter = (*ResponseWriter)(nil)

type ResponseWriter struct {
	logger *zap.Logger

	w http.ResponseWriter
}

func New(logger *zap.Logger, w http.ResponseWriter) ResponseWriter {
	return ResponseWriter{
		logger: logger.Named("http-writer"),
		w:      w,
	}
}

func (h ResponseWriter) WriteResponse(response any) {
	if err := json.NewEncoder(h.w).Encode(response); err != nil {
		h.logger.Info(
			"unable to encode JSON response",
			zap.Error(err),
		)
	}
}
