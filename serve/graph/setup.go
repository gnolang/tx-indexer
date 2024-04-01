package graph

import (
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/serve/graph/generated"
	"github.com/gnolang/tx-indexer/serve/graph/resolvers"
	"github.com/gnolang/tx-indexer/storage"
	"github.com/go-chi/chi/v5"
)

func Setup(s storage.Storage, manager *events.Manager, m *chi.Mux) *chi.Mux {
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(
		generated.Config{Resolvers: resolvers.NewResolver(s, manager)},
	))

	srv.AddTransport(&transport.Websocket{})

	m.Handle("/", playground.Handler("GraphQL playground", "/query"))
	m.Handle("/query", srv)

	return m
}
