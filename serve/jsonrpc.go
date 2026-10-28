package serve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/olahol/melody"
	"go.uber.org/zap"

	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve/budget"
	"github.com/gnolang/tx-indexer/serve/conns"
	"github.com/gnolang/tx-indexer/serve/conns/wsconn"
	"github.com/gnolang/tx-indexer/serve/deadline"
	"github.com/gnolang/tx-indexer/serve/filters"
	"github.com/gnolang/tx-indexer/serve/handlers/block"
	"github.com/gnolang/tx-indexer/serve/handlers/gas"
	"github.com/gnolang/tx-indexer/serve/handlers/subs"
	"github.com/gnolang/tx-indexer/serve/handlers/supply"
	"github.com/gnolang/tx-indexer/serve/handlers/tx"
	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/observe"
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

	// observer records each call served; nil records none
	observer *observe.Observer

	// queryDeadline bounds each request, a batch as a whole; zero leaves
	// requests unbounded
	queryDeadline time.Duration
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

	// Register the POST methodHandler for HTTP requests. The observer's
	// middleware reads the client address before the calls are served
	mux.Method(http.MethodPost, "/", j.observer.Middleware(http.HandlerFunc(j.handleHTTPRequest)))

	// Register the WS methodHandler. The observer's middleware reads the
	// client address of the upgrade request, which every call the connection
	// carries shares
	mux.Handle("/ws", j.observer.Middleware(http.HandlerFunc(j.handleWSRequest)))

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

// RegisterGasPriceEndpoints registers the gas price endpoints
func (j *JSONRPC) RegisterGasEndpoints(db gas.Storage) {
	gasPriceHandler := gas.NewHandler(db)

	j.RegisterHandler(
		"getGasPrice",
		gasPriceHandler.GetGasPriceHandler,
	)
}

// RegisterSupplyEndpoints registers the supply endpoints. The handler is
// shared with the GraphQL layer, so both surfaces serve one snapshot.
func (j *JSONRPC) RegisterSupplyEndpoints(h *supply.Handler) {
	j.RegisterHandler(
		"getSupply",
		h.GetSupplyHandler,
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
		"newTransactionFilter",
		subsHandler.NewTransactionFilterHandler,
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

		// The upgrade request's context lasts as long as the connection does
		ctx := s.Request.Context()
		writer := wsWriter.New(j.logger, s)

		// A refused message is answered under each call's ID, unlike a refused
		// HTTP request: melody caps a message at 512 bytes, so the answer stays
		// small, and a client multiplexing calls over the connection needs
		// their IDs
		end, err := j.chargeMessage(ctx, requests)
		if err != nil {
			writer.WriteResponse(refusalResponses(requests, err))

			return
		}

		defer end()

		// Handle the request
		j.handleRequest(
			ctx,
			metadata.NewMetadata(
				s.RemoteAddr().String(),
				metadata.WithWebSocketID(wsConnID),
			),
			writer,
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
		r.Context(),
		metadata.NewMetadata(r.RemoteAddr),
		httpWriter.New(j.logger, w),
		requests,
	)
}

// WriteRefusal answers a request refused before it was served with a 429 and
// one error, err, naming no call. Its body is left unread, so a refusal costs
// the same whatever the client sent: a batch of a megabyte would otherwise be
// read, parsed and answered one error per call, uncharged.
func (j *JSONRPC) WriteRefusal(w http.ResponseWriter, _ *http.Request, err error) {
	w.Header().Set("Content-Type", jsonMimeType)
	w.WriteHeader(http.StatusTooManyRequests)

	httpWriter.New(j.logger, w).WriteResponse(refusalResponses(nil, err))
}

// refusalResponses answer requests, refused with err, with err in place of
// each call's result, under each call's ID: a batch as a batch, as
// handleRequest answers one, and no request at all with one error and no ID
func refusalResponses(requests spec.BaseJSONRequests, err error) any {
	if len(requests) == 0 {
		requests = spec.BaseJSONRequests{nil}
	}

	refusal := spec.GenerateResponseError(err)
	responses := make(spec.BaseJSONResponses, len(requests))

	for i, request := range requests {
		responses[i] = spec.NewJSONResponse(idOf(request), nil, refusal)
	}

	if len(responses) == 1 {
		return responses[0]
	}

	return responses
}

// chargeMessage charges the calls of a message sent over a WebSocket
// connection, carrying ctx, to the client that opened it, when a time budget
// admitted the connection. It returns the function that ends them, or the
// error that refuses them. A message of subscriptions alone is neither
// charged nor refused: a subscription lives as long as its connection.
func (j *JSONRPC) chargeMessage(ctx context.Context, requests spec.BaseJSONRequests) (func(), error) {
	client, ok := budget.FromContext(ctx)
	if !ok || subscriptionsOnly(requests) {
		return func() {}, nil
	}

	end, err := client.Reserve()
	if err != nil {
		var exhausted *budget.ExhaustedError
		if errors.As(err, &exhausted) {
			j.observer.Refused(client.Address, metrics.APIJSONRPC, metrics.RefusalClientTimeBudget, exhausted.RetryAfter)
		}

		return nil, err
	}

	return end, nil
}

// isSubscriptionMethod reports whether method opens or closes a subscription
func isSubscriptionMethod(method string) bool {
	switch method {
	case "subscribe", "unsubscribe":
		return true
	default:
		return false
	}
}

// subscriptionsOnly reports whether requests open or close subscriptions and
// nothing else
func subscriptionsOnly(requests spec.BaseJSONRequests) bool {
	for _, request := range requests {
		if request == nil || !isSubscriptionMethod(request.Method) {
			return false
		}
	}

	return len(requests) > 0
}

// handleWSRequest handles incoming WS requests. A failed upgrade is logged
// at warn: any client causes one, and error marks the indexer's own failure.
func (j *JSONRPC) handleWSRequest(w http.ResponseWriter, r *http.Request) {
	if err := j.ws.HandleRequest(w, r); err != nil {
		j.logger.Warn(
			"unable to initialize WS connection",
			zap.Error(err),
		)
	}
}

// handleRequest handles the specific requests with a
// custom response writer
func (j *JSONRPC) handleRequest(
	ctx context.Context,
	metadata *metadata.Metadata,
	writer writer.ResponseWriter,
	requests spec.BaseJSONRequests,
) {
	// The calls of a batch share one deadline, so the batch is bounded as a
	// whole and not call by call
	ctx, cancel := deadline.WithTimeout(ctx, j.queryDeadline)
	defer cancel()

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
				idOf(baseRequest),
				nil,
				spec.NewJSONError(
					"invalid JSON-RPC 2.0 request",
					spec.InvalidRequestErrorCode,
				),
			)

			continue
		}

		// Run the method methodHandler
		handleResp, handleErr := j.route(ctx, metadata, baseRequest)
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

// route routes the base request to the appropriate handler. Only a call to
// a registered method is observed: the method then names a known operation,
// never a string the client made up.
func (j *JSONRPC) route(
	ctx context.Context,
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

	ctx, call := j.observer.Start(ctx, metrics.APIJSONRPC, request.Method)

	// The call stays unanswered until its handler returns: a handler that
	// panics never returns, and its call still ends
	answered := observe.ErrUnanswered

	defer func() {
		call.End(ctx, answered)
	}()

	response, err := runHandler(ctx, handler, metadata, request.Params)

	answered = nil
	if err != nil {
		answered = err
	}

	return response, err
}

// runHandler runs handler for a call, unless the call's request has already
// ended
func runHandler(
	ctx context.Context,
	handler Handler,
	metadata *metadata.Metadata,
	params []any,
) (any, *spec.BaseJSONError) {
	// A request that has ended, by its deadline or its client leaving, runs
	// no further calls of its batch: each answers the reason instead
	if ctx.Err() != nil {
		return nil, spec.GenerateResponseError(context.Cause(ctx))
	}

	return handler(ctx, metadata, params)
}

// isValidBaseRequest validates that the base JSON request is valid. A call that
// is null, as a batch can hold, is not.
func isValidBaseRequest(baseRequest *spec.BaseJSONRequest) bool {
	if baseRequest == nil || baseRequest.Method == "" {
		return false
	}

	return baseRequest.JSONRPC == spec.JSONRPCVersion
}

// idOf is the ID of request, 0 for a call that is null
func idOf(request *spec.BaseJSONRequest) uint {
	if request == nil {
		return 0
	}

	return request.ID
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
