package graph

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gnolang/tx-indexer/serve/graph/model"
	"github.com/gnolang/tx-indexer/storage"
)

// testResolver returns a resolver backed by a real store in a temp directory.
// The event manager and supply handler are nil: the list resolvers do not
// reach for them.
func testResolver(t *testing.T) *Resolver {
	t.Helper()

	s, err := storage.NewPebble(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, s.Close())
	})

	return NewResolver(s, nil, nil)
}

// testContext carries the response context that graphql.AddError writes into
func testContext() context.Context {
	return graphql.WithResponseContext(
		context.Background(),
		graphql.DefaultErrorPresenter,
		nil,
	)
}

// seedTxs writes txsPerHeight transactions at every height in [1, to]
func seedTxs(t *testing.T, r *Resolver, to int64, txsPerHeight int) {
	t.Helper()

	wb := r.store.WriteBatch()

	for height := int64(1); height <= to; height++ {
		for index := range txsPerHeight {
			require.NoError(t, wb.SetTx(&types.TxResult{
				Height: height,
				Index:  uint32(index),
				Tx:     types.Tx{byte(height), byte(index)},
			}))
		}
	}

	require.NoError(t, wb.Commit())
}

// seedBlocks writes one block at every height in [1, to]
func seedBlocks(t *testing.T, r *Resolver, to int64) {
	t.Helper()

	wb := r.store.WriteBatch()

	for height := int64(1); height <= to; height++ {
		require.NoError(t, wb.SetBlock(&types.Block{
			Header: types.Header{Height: height},
		}))
	}

	require.NoError(t, wb.Commit())
}

// soleError asserts the resolver attached exactly one error and returns it
func soleError(t *testing.T, ctx context.Context) *gqlerror.Error {
	t.Helper()

	list := graphql.GetErrors(ctx)
	require.Len(t, list, 1)

	return list[0]
}

func TestGetTransactions_UnderCapIsUnchanged(t *testing.T) {
	t.Parallel()

	r := testResolver(t)
	seedTxs(t, r, 10, 3)

	ctx := testContext()

	out, err := r.Query().GetTransactions(ctx, model.FilterTransaction{}, nil, nil)
	require.NoError(t, err)

	assert.Len(t, out, 30)
	assert.Empty(t, graphql.GetErrors(ctx))
}

// A truncated page must end on a block boundary, so that the client can resume
// with from_block_height and neither lose nor refetch a row. The filter cannot
// express a mid-block resume point: the index window applies to every block in
// the range, not just the boundary one.
func TestGetTransactions_TruncationEndsOnBlockBoundary(t *testing.T) {
	t.Parallel()

	r := testResolver(t)

	// 3 txs per height puts the cap inside height 3334, not on a boundary
	seedTxs(t, r, 3400, 3)

	ctx := testContext()

	out, err := r.Query().GetTransactions(ctx, model.FilterTransaction{}, nil, nil)
	require.NoError(t, err)

	require.Len(t, out, 9999)
	assert.Equal(t, 3333, out[len(out)-1].BlockHeight())

	for _, tx := range out {
		require.NotEqual(t, 3334, tx.BlockHeight(), "returned a row from the split block")
	}

	gqlErr := soleError(t, ctx)
	assert.Equal(t, "MAX_ELEMENTS_REACHED", gqlErr.Extensions["code"])
	assert.Equal(t, int64(3333), gqlErr.Extensions["lastCompleteBlockHeight"])
	assert.Equal(t, maxElementsPerQuery, gqlErr.Extensions["maxElementsPerQuery"])
}

// Descending order trims the lowest returned height and resumes downward, so
// the resume point is the last height completed on the way down.
func TestGetTransactions_TruncationEndsOnBlockBoundaryDescending(t *testing.T) {
	t.Parallel()

	r := testResolver(t)
	seedTxs(t, r, 3400, 3)

	ctx := testContext()

	out, err := r.Query().GetTransactions(
		ctx,
		model.FilterTransaction{},
		&model.TransactionOrder{HeightAndIndex: model.OrderDesc},
		nil,
	)
	require.NoError(t, err)

	require.Len(t, out, 9999)
	assert.Equal(t, 68, out[len(out)-1].BlockHeight())

	for _, tx := range out {
		require.NotEqual(t, 67, tx.BlockHeight(), "returned a row from the split block")
	}

	gqlErr := soleError(t, ctx)
	assert.Equal(t, "MAX_ELEMENTS_REACHED", gqlErr.Extensions["code"])
	assert.Equal(t, int64(68), gqlErr.Extensions["lastCompleteBlockHeight"])
}

// A page that walks from the cursor must cover every row exactly once
func TestGetTransactions_ResumingFromTheReportedHeightLosesNothing(t *testing.T) {
	t.Parallel()

	r := testResolver(t)
	seedTxs(t, r, 3400, 3)

	ctx := testContext()

	first, err := r.Query().GetTransactions(ctx, model.FilterTransaction{}, nil, nil)
	require.NoError(t, err)

	lastComplete := int(soleError(t, ctx).Extensions["lastCompleteBlockHeight"].(int64))

	secondCtx := testContext()
	second, err := r.Query().GetTransactions(
		secondCtx,
		model.FilterTransaction{BlockHeight: &model.FilterInt{Gt: &lastComplete}},
		nil,
		nil,
	)
	require.NoError(t, err)

	seen := make(map[string]int, len(first)+len(second))
	for _, tx := range append(append([]*model.Transaction{}, first...), second...) {
		seen[tx.ID()]++
	}

	assert.Len(t, seen, 3400*3, "the two pages do not cover the chain exactly")

	for id, count := range seen {
		require.Equalf(t, 1, count, "row %s returned twice", id)
	}
}

// One height can hold more rows than the cap. Block-granular paging cannot
// express that, so say so rather than hand back an empty page forever.
func TestGetTransactions_SingleHeightExceedsCap(t *testing.T) {
	t.Parallel()

	r := testResolver(t)
	seedTxs(t, r, 1, maxElementsPerQuery+1)

	ctx := testContext()

	out, err := r.Query().GetTransactions(ctx, model.FilterTransaction{}, nil, nil)
	require.NoError(t, err)

	assert.Len(t, out, maxElementsPerQuery)

	gqlErr := soleError(t, ctx)
	assert.Equal(t, "BLOCK_EXCEEDS_MAX_ELEMENTS", gqlErr.Extensions["code"])
	assert.Equal(t, int64(1), gqlErr.Extensions["blockHeight"])
}

// Blocks are one row per height, so a truncated page is always complete
func TestGetBlocks_TruncationReportsResumeHeight(t *testing.T) {
	t.Parallel()

	r := testResolver(t)
	seedBlocks(t, r, maxElementsPerQuery+1)

	ctx := testContext()

	out, err := r.Query().GetBlocks(ctx, model.FilterBlock{}, nil, nil)
	require.NoError(t, err)

	require.Len(t, out, maxElementsPerQuery)

	gqlErr := soleError(t, ctx)
	assert.Equal(t, "MAX_ELEMENTS_REACHED", gqlErr.Extensions["code"])
	assert.Equal(t, int64(maxElementsPerQuery), gqlErr.Extensions["lastCompleteBlockHeight"])
}
