package graph

import (
	"context"
	"fmt"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/gnolang/gno/tm2/pkg/amino"
	bfttypes "github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gnolang/tx-indexer/serve/graph/model"
	"github.com/gnolang/tx-indexer/storage"
)

func TestEffectiveLimit(t *testing.T) {
	t.Parallel()

	i := func(v int) *int { return &v }

	tests := []struct {
		name  string
		limit *int
		want  int
	}{
		{"absent falls back to the cap", nil, maxElementsPerQuery},
		{"a smaller limit is honoured", i(20), 20},
		{"one is honoured", i(1), 1},
		// Zero and negatives are treated as absent rather than rejected: a
		// caller sending limit:0 gets today's behaviour instead of an empty
		// list they did not ask for.
		{"zero is treated as absent", i(0), maxElementsPerQuery},
		{"negative is treated as absent", i(-5), maxElementsPerQuery},
		// The cap is a ceiling, not a default: limit may only lower it.
		{"above the cap is clamped", i(maxElementsPerQuery + 1), maxElementsPerQuery},
		{"exactly the cap", i(maxElementsPerQuery), maxElementsPerQuery},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, effectiveLimit(tt.limit))
		})
	}
}

// seedStore writes count transactions, one per block, each a MsgCall.
func seedStore(t *testing.T, s *storage.Pebble, count int) {
	t.Helper()

	b := s.WriteBatch()
	for i := 0; i < count; i++ {
		tx := &std.Tx{Memo: fmt.Sprintf("tx %d", i)}
		encoded, err := amino.Marshal(tx)
		require.NoError(t, err)

		require.NoError(t, b.SetTx(&bfttypes.TxResult{
			Height: int64(i + 1),
			Index:  0,
			Tx:     encoded,
		}))
	}
	require.NoError(t, b.Commit())
}

// collectErrors gives the resolver a context that captures the GraphQL errors
// it adds, which is how truncation is reported.
func collectErrors(ctx context.Context) (context.Context, func() gqlerror.List) {
	ctx = graphql.WithOperationContext(ctx, &graphql.OperationContext{})
	ctx = graphql.WithResponseContext(ctx, graphql.DefaultErrorPresenter,
		func(_ context.Context, err any) error { return fmt.Errorf("panic: %v", err) })
	return ctx, func() gqlerror.List { return graphql.GetErrors(ctx) }
}

func TestGetTransactionsRespectsLimit(t *testing.T) {
	t.Parallel()

	const stored = 50

	tests := []struct {
		name         string
		limit        *int
		wantCount    int
		wantTruncErr bool
	}{
		{
			name:      "a limit below the number stored returns exactly that many",
			limit:     func() *int { v := 20; return &v }(),
			wantCount: 20,
			// Reaching a limit the caller asked for is the query working, not a
			// truncated answer, so it must not raise the cap error.
			wantTruncErr: false,
		},
		{
			name:         "a limit above what is stored returns everything",
			limit:        func() *int { v := 500; return &v }(),
			wantCount:    stored,
			wantTruncErr: false,
		},
		{
			name:         "no limit returns everything under the cap",
			limit:        nil,
			wantCount:    stored,
			wantTruncErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, err := storage.NewPebble(t.TempDir())
			require.NoError(t, err)
			t.Cleanup(func() { _ = s.Close() })
			seedStore(t, s, stored)

			r := &queryResolver{&Resolver{store: s}}
			ctx, errs := collectErrors(context.Background())

			out, err := r.GetTransactions(ctx, model.FilterTransaction{}, nil, tt.limit)
			require.NoError(t, err)
			require.Len(t, out, tt.wantCount)

			gotErr := len(errs()) > 0
			require.Equal(t, tt.wantTruncErr, gotErr,
				"truncation error presence mismatch; errors: %v", errs())
		})
	}
}
