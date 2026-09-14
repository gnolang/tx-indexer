package supply

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	"github.com/gnolang/gno/tm2/pkg/amino"
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/crypto"
	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/stretchr/testify/require"
)

// benchSizes covers a small sheet up to the gnoland-1 genesis, which holds a
// vesting schedule on about 3.26 million accounts.
var benchSizes = []struct {
	name  string
	count int
}{
	{"10k", 10_000},
	{"1m", 1_000_000},
	{"mainnet", 3_260_000},
}

// genesisRows builds count vesting rows, alternating continuous and delayed
// schedules, the shape of a large genesis balance sheet.
func genesisRows(count int) []gnoland.Balance {
	rows := make([]gnoland.Balance, 0, count)

	for i := range count {
		addr := crypto.AddressFromPreimage(fmt.Appendf(nil, "vester-%d", i))

		schedule := continuousSchedule()
		if i%2 == 1 {
			schedule = delayedSchedule(1000)
		}

		rows = append(rows, vestingBalance(addr, schedule, 1000))
	}

	return rows
}

// heapInUse reports the live heap after a full collection, in bytes.
func heapInUse() float64 {
	runtime.GC()

	var stats runtime.MemStats

	runtime.ReadMemStats(&stats)

	return float64(stats.HeapInuse)
}

func BenchmarkFoldVestingWindows(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(size.name, func(b *testing.B) {
			rows := genesisRows(size.count)

			var (
				windows []std.VestingSchedule
				err     error
			)

			b.ReportAllocs()

			for b.Loop() {
				windows, err = FoldVestingWindows(rows)
			}

			require.NoError(b, err)
			require.Len(b, windows, 2, "one continuous window and one cliff")
		})
	}
}

func BenchmarkRefresh(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(size.name, func(b *testing.B) {
			store := &mockStorage{balances: genesisRows(size.count)}
			h := NewHandler(supplyClient(int64(size.count)*1000), store, WithDenoms([]string{testDenom}))
			require.NoError(b, h.loadWindows())

			// The rows are the bootstrap's, not the handler's: drop them so
			// the heap figure is what the handler keeps alive between
			// refreshes.
			store.balances = nil

			b.ReportAllocs()

			for b.Loop() {
				h.refresh(context.Background())
			}

			// Reported after the loop: the first b.Loop call resets the
			// timer, which also discards user metrics reported before it.
			b.ReportMetric(heapInUse(), "heap-bytes")

			supply, err := h.GetSupply(context.Background(), testDenom)
			require.NoError(b, err)
			require.Positive(b, supply.Locked)
		})
	}
}

// supplyClient answers every bank/supply query with total and records nothing,
// so a benchmark measures the handler alone.
func supplyClient(total int64) *mockClient {
	return &mockClient{
		getStatusFn: func(context.Context) (*core_types.ResultStatus, error) {
			return statusAt(42, 150), nil
		},
		batchQueryFn: func(_ context.Context, _ int64, paths []string) ([]*core_types.ResultABCIQuery, error) {
			results := make([]*core_types.ResultABCIQuery, len(paths))
			for i := range paths {
				results[i] = abciData(amino.MustMarshalJSON(total))
			}

			return results, nil
		},
	}
}
