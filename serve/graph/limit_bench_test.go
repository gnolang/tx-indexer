package graph

import (
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
// With no limit, a caller wanting the twenty most recent transactions has no
// way to say so: the resolver runs until it has maxElementsPerQuery matches, so
// twenty rows cost ten thousand.
//
// go test ./serve/graph -run '^$' -bench BenchmarkGetTransactionsLimit
func BenchmarkGetTransactionsLimit(b *testing.B) {
	const stored = 200_000

	s, err := storage.NewPebble(b.TempDir())
	if err != nil {
		b.Fatalf("open: %v", err)
	}

	defer s.Close()

	batch := s.WriteBatch()

	for i := range stored {
		encoded, err := amino.Marshal(&std.Tx{Memo: fmt.Sprintf("tx %d", i)})
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

	r := NewResolver(s, nil, nil).Query()
	desc := &model.TransactionOrder{HeightAndIndex: model.OrderDesc}

	for _, tc := range []struct {
		limit *int
		order *model.TransactionOrder
		name  string
	}{
		{nil, nil, "no limit"},
		{ptr(20), nil, "limit 20"},
		{ptr(100), nil, "limit 100"},
		{ptr(20), desc, "limit 20 descending"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()

			var rows int

			for b.Loop() {
				out, err := r.GetTransactions(testContext(), model.FilterTransaction{}, tc.order, tc.limit)
				if err != nil {
					b.Fatalf("query: %v", err)
				}

				rows = len(out)
			}

			b.ReportMetric(float64(rows), "rows_returned")
		})
	}
}
