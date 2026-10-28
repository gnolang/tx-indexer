package graph

import (
	"context"
	embed "embed"
	"io/fs"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-chi/chi/v5"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/graph/model"
	"github.com/gnolang/tx-indexer/serve/handlers/supply"
	"github.com/gnolang/tx-indexer/serve/observe"
	"github.com/gnolang/tx-indexer/storage"
)

//go:embed examples/*.gql
var examples embed.FS

// Options configure the GraphQL endpoint Setup mounts. The zero value is an
// endpoint with introspection, no query deadline and nothing recorded.
type Options struct {
	// Observer records each root query field served; nil records none, at no
	// cost to them
	Observer *observe.Observer

	// QueryDeadline bounds each query; zero leaves queries unbounded
	QueryDeadline time.Duration

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

	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
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
	m.Handle("/graphql/query", opts.Observer.Middleware(srv))

	return m
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
