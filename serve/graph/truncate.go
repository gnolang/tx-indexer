package graph

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gnolang/tx-indexer/serve/graph/model"
)

// Codes carried in the extensions of a truncated list response. Clients read
// these, and the sibling height fields, rather than parsing the message text.
const (
	codeMaxElementsReached     = "MAX_ELEMENTS_REACHED"
	codeBlockExceedsMaxElement = "BLOCK_EXCEEDS_MAX_ELEMENTS"
)

// truncateTransactions cuts a capped page back to its last whole block and
// reports the height the caller received in full.
//
// A truncated page has to end on a block boundary because that is the only
// resume point a client can express: the index window of a filter applies to
// every block in the range rather than to the boundary block alone, so
// "everything after this height and index" cannot be written as a filter.
// Cutting back costs the caller one block of refetch and keeps the response
// within the element cap, which exists to bound its size.
func truncateTransactions(
	ctx context.Context,
	out []*model.Transaction,
	descending bool,
) []*model.Transaction {
	// The cap trips only once a row has been appended for every count, so the
	// page is never empty here
	splitHeight := int64(out[len(out)-1].BlockHeight())

	whole := out
	for len(whole) > 0 && int64(whole[len(whole)-1].BlockHeight()) == splitHeight {
		whole = whole[:len(whole)-1]
	}

	if len(whole) == 0 {
		// A single height filled the cap, so there is no whole block to cut
		// back to. Block-granular paging cannot express that, so report it
		// instead of handing back an empty page the caller would loop on.
		graphql.AddError(ctx, &gqlerror.Error{
			Message: fmt.Sprintf(
				"block %d alone reaches max elements per query (%d); "+
					"it cannot be paged at block granularity",
				splitHeight,
				maxElementsPerQuery,
			),
			Extensions: map[string]any{
				"code":                codeBlockExceedsMaxElement,
				"maxElementsPerQuery": maxElementsPerQuery,
				"blockHeight":         splitHeight,
			},
		})

		return out
	}

	// Ascending stops below the split block, descending stops above it
	lastComplete := splitHeight - 1
	if descending {
		lastComplete = splitHeight + 1
	}

	graphql.AddError(ctx, maxElementsError(lastComplete))

	return whole
}

// truncateBlocks reports the resume height of a capped block page. Blocks hold
// one row per height, so the page already ends on a boundary and keeps every
// row it collected.
func truncateBlocks(ctx context.Context, out []*model.Block) {
	graphql.AddError(ctx, maxElementsError(out[len(out)-1].Height()))
}

// maxElementsError names the last height the caller received in full. Resume
// ascending with from_block_height greater than it, descending with
// to_block_height less than it.
func maxElementsError(lastCompleteHeight int64) *gqlerror.Error {
	return &gqlerror.Error{
		Message: fmt.Sprintf(
			"max elements per query reached (%d); complete through block height %d",
			maxElementsPerQuery,
			lastCompleteHeight,
		),
		Extensions: map[string]any{
			"code":                    codeMaxElementsReached,
			"maxElementsPerQuery":     maxElementsPerQuery,
			"lastCompleteBlockHeight": lastCompleteHeight,
		},
	}
}
