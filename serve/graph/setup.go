package graph

import (
	"context"
	embed "embed"
	"io/fs"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-chi/chi/v5"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/graph/model"
	"github.com/gnolang/tx-indexer/storage"
)

//go:embed examples/*.gql
var examples embed.FS

func Setup(s storage.Storage, manager *events.Manager, m *chi.Mux) *chi.Mux {
	srv := handler.NewDefaultServer(NewExecutableSchema(
		Config{
			Resolvers: NewResolver(s, manager),
			Directives: DirectiveRoot{
				Filterable: func(ctx context.Context, obj interface{}, next graphql.Resolver, extras []model.FilterableExtra) (res interface{}, err error) {
					return next(ctx)
				},
			},
		},
	))

	srv.AddTransport(&transport.Websocket{})

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
