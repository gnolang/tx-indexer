package graph

import (
	"context"
	embed "embed"
	"io/fs"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.uber.org/zap"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/graph/model"
	"github.com/gnolang/tx-indexer/serve/handlers/supply"
	"github.com/gnolang/tx-indexer/serve/observe"
	"github.com/gnolang/tx-indexer/storage"
)

//go:embed examples/*.gql
var examples embed.FS

// Options configure the GraphQL endpoint Setup mounts. The zero value is an
// endpoint with introspection, no query deadline and nothing recorded or
// logged.
type Options struct {
	// Observer records each root query field served; nil records none, at no
	// cost to them
	Observer *observe.Observer

	// Logger logs what goes wrong in the endpoint, a resolver's panic among
	// it; nil logs nothing
	Logger *zap.Logger

	// QueryDeadline bounds each query; zero leaves queries unbounded
	QueryDeadline time.Duration

	// TokenLimit is the most tokens a document may hold, refused before it
	// is validated, whose cost grows with the document; zero leaves documents
	// unbounded
	TokenLimit int

	// DisableIntrospection refuses introspection queries
	DisableIntrospection bool
}

// Setup mounts the GraphQL playground and endpoint on m, as opts configure them
func Setup(
	s storage.Storage,
	manager *events.Manager,
	sup *supply.Handler,
	m *chi.Mux,
	opts Options,
) *chi.Mux {
	srv := handler.New(NewExecutableSchema(
		Config{
			Resolvers: NewResolver(s, manager, sup),
			Directives: DirectiveRoot{
				Filterable: func(
					ctx context.Context,
					_ interface{},
					next graphql.Resolver,
					_ []model.FilterableExtra,
				) (interface{}, error) {
					return next(ctx)
				},
			},
		},
	))

	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	srv.SetRecoverFunc(recoverFunc(logger))

	if opts.TokenLimit > 0 {
		srv.SetParserTokenLimit(opts.TokenLimit)
	}

	srv.SetValidationRulesFn(cappedRules)

	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		// A failed upgrade is answered gorilla's status, 403 for a refused
		// origin (RFC 6455 §10.2) or 405 for a method other than GET, naming
		// the WebSocket version gorilla speaks (§4.4), and gqlgen's body:
		// gqlgen's own status follows, superfluous, and hijackAware drops it
		// where the HTTP server would log it at error for any client
		Upgrader: websocket.Upgrader{
			Error: func(w http.ResponseWriter, _ *http.Request, status int, _ error) {
				w.Header().Set("Sec-Websocket-Version", "13")
				w.WriteHeader(status)
			},
		},
	})
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	if !opts.DisableIntrospection {
		srv.Use(extension.Introspection{})
	}

	srv.Use(deadlineExtension{deadline: opts.QueryDeadline})

	srv.Use(budgetExtension{observer: opts.Observer})

	// With nothing to record, every field is spared the extension
	if opts.Observer != nil {
		srv.Use(observeExtension{observer: opts.Observer})
	}

	es, err := examplesToSlice()
	if err != nil {
		panic(err)
	}

	m.Handle("/graphql", HandlerWithDefaultTabs("Gno Indexer: GraphQL playground", "/graphql/query", es))
	// The observer reads the client address of each request, and of each
	// WebSocket connection's upgrade request, before its fields are served
	m.Handle("/graphql/query", opts.Observer.Middleware(hijackAware(srv)))

	return m
}

// recoverFunc answers a resolver's panic with "internal system error", as
// gqlgen's own recover function does, and logs it at error through logger,
// where gqlgen's prints it and its stack on stderr. The indexer's logger
// records the stack of an error line.
func recoverFunc(logger *zap.Logger) graphql.RecoverFunc {
	return func(_ context.Context, recovered any) error {
		logger.Error("resolver panicked", zap.Any("panic", recovered))

		return gqlerror.Errorf("internal system error")
	}
}

func examplesToSlice() ([]string, error) {
	var out []string

	//nolint:staticcheck // The overwrite is fine
	err := fs.WalkDir(examples, ".", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		content, err := examples.ReadFile(path)
		if err != nil {
			return err
		}

		out = append(out, string(content))

		return nil
	})

	return out, err
}
