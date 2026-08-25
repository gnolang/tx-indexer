package graph

import (
	"context"
	"fmt"
	"testing"

	"github.com/gnolang/gno/tm2/pkg/amino"
	bfttypes "github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"

	"github.com/gnolang/tx-indexer/serve/graph/model"
	"github.com/gnolang/tx-indexer/storage"
)

// BenchmarkGetTransactionsLimit shows what asking for a page costs.
//
// Before `limit`, a caller wanting the twenty most recent transactions had no
// way to say so: the resolver ran until it had maxElementsPerQuery matches, so
// twenty rows cost ten thousand.
//
// go test ./serve/graph -bench BenchmarkGetTransactionsLimit -benchtime 3x
func BenchmarkGetTransactionsLimit(b *testing.B) {
	const stored = 200_000

	s, err := storage.NewPebble(b.TempDir())
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	defer s.Close()

	batch := s.WriteBatch()
	for i := 0; i < stored; i++ {
		tx := &std.Tx{Memo: fmt.Sprintf("tx %d", i)}
		encoded, err := amino.Marshal(tx)
		if err != nil {
			b.Fatalf("marshal: %v", err)
		}
		if err := batch.SetTx(&bfttypes.TxResult{Height: int64(i + 1), Tx: encoded}); err != nil {
			b.Fatalf("set: %v", err)
		}
		if i%5000 == 0 {
			if err := batch.Commit(); err != nil {
				b.Fatalf("commit: %v", err)
			}
			batch = s.WriteBatch()
		}
	}
	if err := batch.Commit(); err != nil {
		b.Fatalf("commit: %v", err)
	}

	r := &queryResolver{&Resolver{store: s}}
	lim := func(v int) *int { return &v }

	for _, tc := range []struct {
		name  string
		limit *int
	}{
		{"no limit (today)", nil},
		{"limit 20", lim(20)},
		{"limit 100", lim(100)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				ctx, _ := collectErrors(context.Background())
				out, err := r.GetTransactions(ctx, model.FilterTransaction{}, nil, tc.limit)
				if err != nil {
					b.Fatalf("query: %v", err)
				}
				b.ReportMetric(float64(len(out)), "rows_returned")
			}
		})
	}
}
