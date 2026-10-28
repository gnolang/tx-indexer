package ws

import (
	"encoding/json"

	"github.com/gnolang/tx-indexer/serve/writer"
	"github.com/olahol/melody"
	"go.uber.org/zap"
)

var _ writer.ResponseWriter = (*ResponseWriter)(nil)

type ResponseWriter struct {
	logger *zap.Logger

	s *melody.Session
}

func New(logger *zap.Logger, s *melody.Session) ResponseWriter {
	return ResponseWriter{
		logger: logger.Named("ws-writer"),
		s:      s,
	}
}

func (w ResponseWriter) WriteResponse(response any) {
	jsonRaw, encodeErr := json.Marshal(response)
	if encodeErr != nil {
		w.logger.Error(
			"unable to encode JSON-RPC response",
			zap.Error(encodeErr),
		)

		return
	}

	// melody refuses a write to a session that is closed, as one is once its
	// client goes: the client alone causes it, so it is logged at warn
	if err := w.s.Write(jsonRaw); err != nil {
		w.logger.Warn(
			"unable to write WS response",
			zap.Error(err),
		)
	}
}
