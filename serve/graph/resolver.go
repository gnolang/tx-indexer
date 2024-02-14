//go:generate go run github.com/99designs/gqlgen generate

package graph

import (
	"time"

	"github.com/gnolang/tx-indexer/storage"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

const maxElementsPerQuery = 10000

func dereferenceInt(i *int) int {
	if i == nil {
		return 0
	}

	return *i
}

func dereferenceTime(i *time.Time) time.Time {
	if i == nil {
		var t time.Time

		return t
	}

	return *i
}

type Resolver struct {
	store storage.Storage
}

func NewResolver(s storage.Storage) *Resolver {
	return &Resolver{store: s}
}
