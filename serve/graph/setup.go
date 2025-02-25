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
	"github.com/gnolang/tx-indexer/storage"
)

//go:embed examples/*.gql
var examples embed.FS

func Setup(s storage.Storage, manager *events.Manager, m *chi.Mux, disableIntrospection bool) *chi.Mux {
	srv := handler.New(NewExecutableSchema(
		Config{
			Resolvers: NewResolver(s, manager),
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

	if !disableIntrospection {
		srv.Use(extension.Introspection{})
	}

	es, err := examplesToSlice()
	if err != nil {
		panic(err)
	}

	m.Handle("/graphql", HandlerWithDefaultTabs("Gno Indexer: GraphQL playground", "/graphql/query", es))
	m.Handle("/graphql/query", srv)

	return m
}

func examplesToSlice() ([]string, error) {
	var out []string

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
