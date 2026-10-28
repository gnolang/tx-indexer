package graph

import (
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gnolang/tx-indexer/serve/graph/model"
)

func ptr[T any](v T) *T {
	return &v
}

func TestEffectiveLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		limit      *int
		name       string
		want       int
		wantCapped bool
		wantErr    bool
	}{
		{nil, "absent falls back to the cap", maxElementsPerQuery, true, false},
		{ptr(20), "a smaller limit is honoured", 20, false, false},
		{ptr(1), "one is honoured", 1, false, false},
		// The argument is new, so there is no caller to stay compatible with:
		// a limit below 1 is a mistake, and answering it with the cap hides it
		{ptr(0), "zero is rejected", 0, false, true},
		{ptr(-5), "negative is rejected", 0, false, true},
		// The cap is a ceiling, not a default: limit may only lower it
		{ptr(maxElementsPerQuery + 1), "above the cap is capped", maxElementsPerQuery, true, false},
		// Asking for exactly the cap is still the caller's own limit
		{ptr(maxElementsPerQuery), "exactly the cap", maxElementsPerQuery, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, capped, err := effectiveLimit(tt.limit)
			if tt.wantErr {
				require.ErrorContains(t, err, "limit must be at least 1")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantCapped, capped)
		})
	}
}

// limitCase is one query against a store seeded with one row per height in
// [1, stored], so the heights returned name the rows returned
type limitCase struct {
	limit     *int
	name      string
	wantCode  string // empty when the page must carry no error
	stored    int
	wantLen   int
	wantFirst int
	wantLast  int
	desc      bool
	sparse    bool // match heights 3, 7 and 9 only
}

var limitCases = []limitCase{
	{
		name:   "a limit below the number stored returns exactly that many",
		limit:  ptr(20),
		stored: 50, wantLen: 20, wantFirst: 1, wantLast: 20,
	},
	{
		name:   "a limit above what is stored returns everything",
		limit:  ptr(500),
		stored: 50, wantLen: 50, wantFirst: 1, wantLast: 50,
	},
	{
		name:   "no limit returns everything under the cap",
		stored: 50, wantLen: 50, wantFirst: 1, wantLast: 50,
	},
	{
		name:   "descending with a limit returns the top heights",
		limit:  ptr(5),
		desc:   true,
		stored: 50, wantLen: 5, wantFirst: 50, wantLast: 46,
	},
	{
		// limit counts matches, not the rows scanned to find them
		name:   "a sparse filter with a limit returns that many matches",
		limit:  ptr(2),
		sparse: true,
		stored: 50, wantLen: 2, wantFirst: 3, wantLast: 7,
	},
	{
		name:   "exactly the cap matching and no limit is not a truncation",
		stored: maxElementsPerQuery, wantLen: maxElementsPerQuery,
		wantFirst: 1, wantLast: maxElementsPerQuery,
	},
	{
		name:   "a limit equal to the cap is the caller's own limit",
		limit:  ptr(maxElementsPerQuery),
		stored: maxElementsPerQuery + 1, wantLen: maxElementsPerQuery,
		wantFirst: 1, wantLast: maxElementsPerQuery,
	},
}

// cappedCases are cut short by the hard cap. One row per height puts the cut
// on a block boundary for blocks, and inside the last height for
// transactions, which truncateTransactions then drops: wantLast is the last
// height a caller receives in full.
func cappedCases(wantLast int) []limitCase {
	return []limitCase{
		{
			name:   "one past the cap with no limit raises the cap error",
			stored: maxElementsPerQuery + 1, wantLen: wantLast,
			wantFirst: 1, wantLast: wantLast, wantCode: codeMaxElementsReached,
		},
		{
			name:   "a limit above the cap is capped and raises the cap error",
			limit:  ptr(maxElementsPerQuery + 1),
			stored: maxElementsPerQuery + 1, wantLen: wantLast,
			wantFirst: 1, wantLast: wantLast, wantCode: codeMaxElementsReached,
		},
	}
}

// seededResolvers seeds one store per distinct size the cases ask for, so the
// cases at the cap share their ten thousand rows
func seededResolvers(
	t *testing.T,
	cases []limitCase,
	seed func(*testing.T, *Resolver, int),
) map[int]*Resolver {
	t.Helper()

	out := make(map[int]*Resolver)

	for _, tc := range cases {
		if _, ok := out[tc.stored]; ok {
			continue
		}

		r := testResolver(t)
		seed(t, r, tc.stored)
		out[tc.stored] = r
	}

	return out
}

// sparseHeights are the only heights a sparse case matches, out of a range the
// storage scans in full, so a resolver counting scanned rows instead of
// matches stops early
var sparseHeights = []int{3, 7, 9}

func assertLimitPage(t *testing.T, tc limitCase, heights []int, errs gqlerror.List, err error) {
	t.Helper()

	require.NoError(t, err)
	require.Len(t, heights, tc.wantLen)
	assert.Equal(t, tc.wantFirst, heights[0])
	assert.Equal(t, tc.wantLast, heights[len(heights)-1])

	if tc.wantCode == "" {
		assert.Empty(t, errs)

		return
	}

	require.Len(t, errs, 1)
	assert.Equal(t, tc.wantCode, errs[0].Extensions["code"])
	assert.Equal(t, int64(tc.wantLast), errs[0].Extensions["lastCompleteBlockHeight"])
}

func TestGetTransactions_Limit(t *testing.T) {
	t.Parallel()

	cases := append(append([]limitCase{}, limitCases...), cappedCases(maxElementsPerQuery-1)...)
	stores := seededResolvers(t, cases, func(t *testing.T, r *Resolver, stored int) {
		t.Helper()
		seedTxs(t, r, int64(stored), 1)
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var where model.FilterTransaction

			if tc.sparse {
				for _, h := range sparseHeights {
					where.Or = append(where.Or, &model.FilterTransaction{BlockHeight: &model.FilterInt{Eq: ptr(h)}})
				}
			}

			var order *model.TransactionOrder
			if tc.desc {
				order = &model.TransactionOrder{HeightAndIndex: model.OrderDesc}
			}

			ctx := testContext()
			out, err := stores[tc.stored].Query().GetTransactions(ctx, where, order, tc.limit)

			heights := make([]int, 0, len(out))
			for _, tx := range out {
				heights = append(heights, tx.BlockHeight())
			}

			assertLimitPage(t, tc, heights, graphql.GetErrors(ctx), err)
		})
	}
}

func TestGetBlocks_Limit(t *testing.T) {
	t.Parallel()

	cases := append(append([]limitCase{}, limitCases...), cappedCases(maxElementsPerQuery)...)
	stores := seededResolvers(t, cases, func(t *testing.T, r *Resolver, stored int) {
		t.Helper()
		seedBlocks(t, r, int64(stored))
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var where model.FilterBlock

			if tc.sparse {
				for _, h := range sparseHeights {
					where.Or = append(where.Or, &model.FilterBlock{Height: &model.FilterInt{Eq: ptr(h)}})
				}
			}

			var order *model.BlockOrder
			if tc.desc {
				order = &model.BlockOrder{Height: model.OrderDesc}
			}

			ctx := testContext()
			out, err := stores[tc.stored].Query().GetBlocks(ctx, where, order, tc.limit)

			heights := make([]int, 0, len(out))
			for _, b := range out {
				heights = append(heights, int(b.Height()))
			}

			assertLimitPage(t, tc, heights, graphql.GetErrors(ctx), err)
		})
	}
}

func TestLimitBelowOneIsRejected(t *testing.T) {
	t.Parallel()

	r := testResolver(t)
	seedTxs(t, r, 5, 1)
	seedBlocks(t, r, 5)

	for _, limit := range []int{0, -1} {
		txs, err := r.Query().GetTransactions(testContext(), model.FilterTransaction{}, nil, ptr(limit))
		require.ErrorContains(t, err, "limit must be at least 1")
		assert.Nil(t, txs)

		blocks, err := r.Query().GetBlocks(testContext(), model.FilterBlock{}, nil, ptr(limit))
		require.ErrorContains(t, err, "limit must be at least 1")
		assert.Nil(t, blocks)
	}
}
