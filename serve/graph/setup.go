package graph

import (
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/storage"
	"github.com/go-chi/chi/v5"
)

func Setup(s storage.Storage, manager *events.Manager, m *chi.Mux) *chi.Mux {
	srv := handler.NewDefaultServer(NewExecutableSchema(
		Config{Resolvers: NewResolver(s, manager)},
	))

	srv.AddTransport(&transport.Websocket{})

	m.Handle("/graphql", playground.Handler("GraphQL playground", "/query"))
	m.Handle("/graphql/query", srv)

	return m
}
