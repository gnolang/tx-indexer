package graph

import (
	"testing"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/gnolang/tx-indexer/events"
	"github.com/gnolang/tx-indexer/storage"
)

// panickingStore panics on every read of a block range
type panickingStore struct {
	*slowStore
}

// BlockIterator panics
func (panickingStore) BlockIterator(uint64, uint64) (storage.Iterator[*types.Block], error) {
	panic("the store failed")
}

// BlockReverseIterator panics
func (panickingStore) BlockReverseIterator(uint64, uint64) (storage.Iterator[*types.Block], error) {
	panic("the store failed")
}

// A resolver's panic is answered "internal system error", as gqlgen answers
// it, and logged at error with its stack through the endpoint's logger, not
// printed on stderr
func TestSetup_LogsAResolverPanic(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core, zap.AddStacktrace(zap.ErrorLevel))

	mux := Setup(panickingStore{&slowStore{}}, events.NewManager(), nil, chi.NewMux(), Options{Logger: logger})

	resp := postQuery(t, mux, listQueries["getBlocks"])
	require.Len(t, resp.Errors, 1)
	assert.Equal(t, "internal system error", resp.Errors[0].Message)

	panics := logs.FilterLevelExact(zap.ErrorLevel).All()
	require.Len(t, panics, 1)

	assert.Equal(t, "the store failed", panics[0].ContextMap()["panic"])
	assert.Contains(t, panics[0].Stack, "panickingStore")
}
