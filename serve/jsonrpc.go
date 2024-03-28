package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/olahol/melody"
	"go.uber.org/zap"

	"github.com/gnolang/tx-indexer/serve/conns"
	"github.com/gnolang/tx-indexer/serve/conns/wsconn"
	"github.com/gnolang/tx-indexer/serve/filters"
	"github.com/gnolang/tx-indexer/serve/handlers/block"
	"github.com/gnolang/tx-indexer/serve/handlers/subs"
	"github.com/gnolang/tx-indexer/serve/handlers/tx"
	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/spec"
	"github.com/gnolang/tx-indexer/serve/writer"
	httpWriter "github.com/gnolang/tx-indexer/serve/writer/http"
	wsWriter "github.com/gnolang/tx-indexer/serve/writer/ws"
	"github.com/gnolang/tx-indexer/storage"
)

const (
	jsonMimeType       = "application/json" // Only JSON is supported
	maxRequestBodySize = 1 << 20            // 1MB
	wsIDKey            = "ws-id"            // key used for WS connection metadata
)

// maxSizeMiddleware enforces a 1MB size limit on the request body
func maxSizeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

		next.ServeHTTP(w, r)
	})
}

// JSONRPC is the JSONRPC server instance, that is capable
// of handling both HTTP and WS requests
type JSONRPC struct {
	// wsConns keeps track of WS connections
	// that need to be directly accessed by certain methods
	wsConns conns.ConnectionManager

	// events keeps track of event subscriptions
	// that are used within the JSON-RPC server
	events Events

	logger *zap.Logger

	// handlers are the registered method handlers
	handlers handlers

	// ws handles incoming and active WS connections
	ws *melody.Melody
}

// NewJSONRPC creates a new instance of the JSONRPC server
func NewJSONRPC(events Events, opts ...Option) *JSONRPC {
	j := &JSONRPC{
		logger:   zap.NewNop(),
		handlers: newHandlers(),
		ws:       melody.New(),
		events:   events,
	}

	for _, opt := range opts {
		opt(j)
	}

	// Set up the WS connection manager
	j.wsConns = wsconn.NewConns(j.logger)

	// Set up the WS listeners
	j.setupWSListeners()

	return j
}

// SetupRoutes sets up the request router for the indexer service
func (j *JSONRPC) SetupRoutes(mux *chi.Mux) *chi.Mux {
	// Set up the middlewares
	mux.Use(middleware.AllowContentType(jsonMimeType))
	mux.Use(maxSizeMiddleware)

	// OPTIONS requests are ignored
	mux.Options("/", func(http.ResponseWriter, *http.Request) {})

	// Register the POST methodHandler for HTTP requests
	mux.Post("/", j.handleHTTPRequest)

	// Register the WS methodHandler
	mux.HandleFunc("/ws", j.handleWSRequest)

	return mux
}

// RegisterHandler registers a new method handler,
// overwriting existing ones, if any
func (j *JSONRPC) RegisterHandler(method string, handler Handler) {
	j.handlers.addHandler(method, handler)
}

// UnregisterHandler removes the method handler for the specified method, if any
func (j *JSONRPC) UnregisterHandler(method string) {
	j.handlers.removeHandler(method)
}

// RegisterTxEndpoints registers the transaction endpoints
func (j *JSONRPC) RegisterTxEndpoints(db tx.Storage) {
	txHandler := tx.NewHandler(db)

	j.RegisterHandler(
		"getTxResult",
		txHandler.GetTxHandler,
	)

	j.RegisterHandler(
		"getTxResultByHash",
		txHandler.GetTxByHashHandler,
	)
}

// RegisterBlockEndpoints registers the block endpoints
func (j *JSONRPC) RegisterBlockEndpoints(db block.Storage) {
	blockHandler := block.NewHandler(db)

	j.RegisterHandler(
		"getBlock",
		blockHandler.GetBlockHandler,
	)
}

func (j *JSONRPC) RegisterSubEndpoints(db storage.Storage) {
	fm := filters.NewFilterManager(context.Background(), db, j.events)

	subsHandler := subs.NewHandler(
		fm,
		j.wsConns,
	)

	j.RegisterHandler(
		"subscribe",
		subsHandler.SubscribeHandler,
	)

	j.RegisterHandler(
		"unsubscribe",
		subsHandler.UnsubscribeHandler,
	)

	j.RegisterHandler(
		"newBlockFilter",
		subsHandler.NewBlockFilterHandler,
	)

	j.RegisterHandler(
		"getFilterChanges",
		subsHandler.GetFilterChangesHandler,
	)

	j.RegisterHandler(
		"uninstallFilter",
		subsHandler.UninstallFilterHandler,
	)
}

// setupWSListeners sets up handlers for WS events
func (j *JSONRPC) setupWSListeners() {
	// Set up the new connection handler
	j.ws.HandleConnect(func(s *melody.Session) {
		j.logger.Info(
			"WS connection established",
			zap.String("remote", s.RemoteAddr().String()),
		)

		// Generate the WS ID
		wsID := uuid.NewString()
		s.Set(wsIDKey, wsID)

		// Register the connection so it's queryable
		j.wsConns.AddWSConnection(wsID, s)
	})

	// Set up the connection disconnect handler
	j.ws.HandleDisconnect(func(s *melody.Session) {
		j.logger.Info(
			"WS connection terminated",
			zap.String("remote", s.RemoteAddr().String()),
		)

		// Read the WS ID
		wsIDRaw, _ := s.Get(wsIDKey)
		wsConnID := wsIDRaw.(string)

		// Remove the WS connection
		j.wsConns.RemoveWSConnection(wsConnID)
	})

	// Set up the core message method handler
	j.ws.HandleMessage(func(s *melody.Session, msg []byte) {
		// Extract the base request
		requests, err := extractBaseRequests(msg)
		if err != nil {
			// Malformed requests are completely ignored
			return
		}

		// Get the ID associated with this active WS connection
		wsIDRaw, _ := s.Get(wsIDKey)
		wsConnID := wsIDRaw.(string)

		// Handle the request
		j.handleRequest(
			metadata.NewMetadata(
				s.RemoteAddr().String(),
				metadata.WithWebSocketID(wsConnID),
			),
			wsWriter.New(j.logger, s),
			requests,
		)
	})
}

// handleHTTPRequest handles incoming HTTP requests
func (j *JSONRPC) handleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	requestBody, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		http.Error(
			w,
			"unable to read request",
			http.StatusBadRequest,
		)

		return
	}

	requests, err := extractBaseRequests(requestBody)
	if err != nil {
		http.Error(
			w,
			"Invalid request body",
			http.StatusBadRequest,
		)

		return
	}

	// Handle the request
	w.Header().Set("Content-Type", jsonMimeType)
	j.handleRequest(
		metadata.NewMetadata(r.RemoteAddr),
		httpWriter.New(j.logger, w),
		requests,
	)
}

// handleWSRequest handles incoming WS requests
func (j *JSONRPC) handleWSRequest(w http.ResponseWriter, r *http.Request) {
	if err := j.ws.HandleRequest(w, r); err != nil {
		j.logger.Error(
			"unable to initialize WS connection",
			zap.Error(err),
		)
	}
}

// handleRequest handles the specific requests with a
// custom response writer
func (j *JSONRPC) handleRequest(
	metadata *metadata.Metadata,
	writer writer.ResponseWriter,
	requests spec.BaseJSONRequests,
) {
	// Parse all JSON-RPC requests
	responses := make(spec.BaseJSONResponses, len(requests))

	for i, baseRequest := range requests {
		// Log the request
		j.logger.Debug(
			"incoming request",
			zap.Any("request", baseRequest),
		)

		// Make sure it's a valid base request
		if !isValidBaseRequest(baseRequest) {
			// Marshal the JSON-RPC error
			responses[i] = spec.NewJSONResponse(
				baseRequest.ID,
				nil,
				spec.NewJSONError(
					"invalid JSON-RPC 2.0 request",
					spec.InvalidRequestErrorCode,
				),
			)

			continue
		}

		// Run the method methodHandler
		handleResp, handleErr := j.route(metadata, baseRequest)
		if handleErr != nil {
			j.logger.Debug(
				"unable to handle JSON-RPC request",
				zap.Any("request", baseRequest),
				zap.Any("error", handleErr),
			)

			responses[i] = spec.NewJSONResponse(
				baseRequest.ID,
				nil,
				handleErr,
			)

			continue
		}

		j.logger.Debug(
			"handled request",
			zap.Any("request", baseRequest),
		)

		responses[i] = spec.NewJSONResponse(
			baseRequest.ID,
			handleResp,
			nil,
		)
	}

	if len(responses) == 1 {
		// Write the JSON response as a single response
		writer.WriteResponse(responses[0])

		return
	}

	// Write the JSON response as a batch
	writer.WriteResponse(responses)
}

// route routes the base request to the appropriate handler
func (j *JSONRPC) route(
	metadata *metadata.Metadata,
	request *spec.BaseJSONRequest,
) (any, *spec.BaseJSONError) {
	// Get the appropriate handler
	handler := j.handlers[request.Method]
	if handler == nil {
		return nil, spec.NewJSONError(
			"Method handler not set",
			spec.MethodNotFoundErrorCode,
		)
	}

	return handler(metadata, request.Params)
}

// isValidBaseRequest validates that the base JSON request is valid
func isValidBaseRequest(baseRequest *spec.BaseJSONRequest) bool {
	if baseRequest.Method == "" {
		return false
	}

	return baseRequest.JSONRPC == spec.JSONRPCVersion
}

// extractBaseRequests extracts the base JSON-RPC request from the
// request body
func extractBaseRequests(requestBody []byte) (spec.BaseJSONRequests, error) {
	// Extract the request
	var requests spec.BaseJSONRequests

	// Check if the request is a batch request
	if err := json.Unmarshal(requestBody, &requests); err != nil {
		// Try to get a single JSON-RPC request, since this is not a batch
		var baseRequest *spec.BaseJSONRequest
		if err := json.Unmarshal(requestBody, &baseRequest); err != nil {
			return nil, err
		}

		requests = spec.BaseJSONRequests{
			baseRequest,
		}
	}

	return requests, nil
}
