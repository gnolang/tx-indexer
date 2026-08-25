package supply

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	"github.com/gnolang/gno/tm2/pkg/amino"
	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/crypto"
	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/methods"
)

// testDenom stands in for the chain's gas denom.
const testDenom = "ugnot"

// ---------------------------------------------------------------
// Mocks

type (
	getStatusDelegate  func(context.Context) (*core_types.ResultStatus, error)
	batchQueryDelegate func(context.Context, int64, []string) ([]*core_types.ResultABCIQuery, error)
)

type mockClient struct {
	batchQueryFn batchQueryDelegate
	getStatusFn  getStatusDelegate
}

func (m *mockClient) GetStatus(ctx context.Context) (*core_types.ResultStatus, error) {
	return m.getStatusFn(ctx)
}

func (m *mockClient) ABCIQueryBatchAtHeight(
	ctx context.Context,
	height int64,
	paths []string,
) ([]*core_types.ResultABCIQuery, error) {
	return m.batchQueryFn(ctx, height, paths)
}

// batchRecorder records the heights and paths every batch was run with.
type batchRecorder struct {
	batches []recordedBatch
	mu      sync.Mutex
}

type recordedBatch struct {
	paths  []string
	height int64
}

func (r *batchRecorder) record(height int64, paths []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.batches = append(r.batches, recordedBatch{height: height, paths: paths})
}

func (r *batchRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.batches)
}

// ---------------------------------------------------------------
// Fixtures

var (
	vesterA = crypto.AddressFromPreimage([]byte("vester-a"))
	vesterB = crypto.AddressFromPreimage([]byte("vester-b"))
	vesterC = crypto.AddressFromPreimage([]byte("vester-c"))
	plain   = crypto.AddressFromPreimage([]byte("plain"))
)

func coin(denom string, amount int64) std.Coin {
	return std.NewCoin(denom, amount)
}

// continuousSchedule vests 1000 of the test denom linearly from t=100 to t=200.
func continuousSchedule() *std.VestingSchedule {
	return &std.VestingSchedule{
		OriginalVesting: std.NewCoins(coin(testDenom, 1000)),
		StartTime:       100,
		EndTime:         200,
	}
}

// delayedSchedule vests denom fully at t=200 (cliff).
func delayedSchedule(denom string, amount int64) *std.VestingSchedule {
	return &std.VestingSchedule{
		OriginalVesting: std.NewCoins(coin(denom, amount)),
		StartTime:       0,
		EndTime:         200,
		Type:            std.VestingDelayed,
	}
}

// vestingBalance is a genesis row that vests amount of the test denom.
func vestingBalance(addr crypto.Address, schedule *std.VestingSchedule, amount int64) gnoland.Balance {
	return gnoland.Balance{
		Address: addr,
		Amount:  std.NewCoins(coin(testDenom, amount)),
		Vesting: schedule,
	}
}

// plainBalance is a genesis row without a vesting schedule.
func plainBalance(addr crypto.Address, amount int64) gnoland.Balance {
	return gnoland.Balance{
		Address: addr,
		Amount:  std.NewCoins(coin(testDenom, amount)),
	}
}

func statusAt(height, unix int64) *core_types.ResultStatus {
	return &core_types.ResultStatus{
		SyncInfo: core_types.SyncInfo{
			LatestBlockHeight: height,
			LatestBlockTime:   time.Unix(unix, 0),
		},
	}
}

// chainFixture answers one batch at one height, from plain maps.
// errBatch fails the whole batch when set; errSupplyResponse makes every
// bank/supply result carry an ABCI error (a chain without the route);
// panicBatch makes the batch call panic.
type chainFixture struct {
	supply   map[string]int64
	balances map[crypto.Address]std.Coins
	rec      *batchRecorder
	status   *core_types.ResultStatus

	errBatch          error
	errSupplyResponse error

	mu         sync.Mutex
	panicBatch bool
}

func newChainFixture() *chainFixture {
	return &chainFixture{
		supply:   map[string]int64{},
		balances: map[crypto.Address]std.Coins{},
		rec:      &batchRecorder{},
		status:   statusAt(42, 150),
	}
}

func (f *chainFixture) client() *mockClient {
	return &mockClient{
		getStatusFn: func(context.Context) (*core_types.ResultStatus, error) {
			f.mu.Lock()
			defer f.mu.Unlock()

			return f.status, nil
		},
		batchQueryFn: func(_ context.Context, height int64, paths []string) ([]*core_types.ResultABCIQuery, error) {
			f.mu.Lock()
			defer f.mu.Unlock()

			f.rec.record(height, paths)

			if f.panicBatch {
				panic("simulated decoder panic")
			}

			if f.errBatch != nil {
				return nil, f.errBatch
			}

			results := make([]*core_types.ResultABCIQuery, len(paths))

			for i, path := range paths {
				switch {
				case hasPrefix(path, "bank/supply/"):
					if f.errSupplyResponse != nil {
						results[i] = abciError(f.errSupplyResponse)

						continue
					}

					results[i] = abciData(amino.MustMarshalJSON(f.supply[path[len("bank/supply/"):]]))
				case hasPrefix(path, "bank/balances/"):
					addr, err := crypto.AddressFromBech32(path[len("bank/balances/"):])
					if err != nil {
						results[i] = abciError(fmt.Errorf("bad address in fixture path %q", path))

						continue
					}

					results[i] = abciData(amino.MustMarshalJSON(f.balances[addr]))
				default:
					results[i] = abciError(fmt.Errorf("unknown query path %q", path))
				}
			}

			return results, nil
		},
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) > len(prefix) && s[:len(prefix)] == prefix
}

func abciData(bz []byte) *core_types.ResultABCIQuery {
	return &core_types.ResultABCIQuery{
		Response: abci.ResponseQuery{
			ResponseBase: abci.ResponseBase{Data: bz},
		},
	}
}

func abciError(err error) *core_types.ResultABCIQuery {
	return &core_types.ResultABCIQuery{
		Response: abci.ResponseQuery{
			ResponseBase: abci.ResponseBase{Error: abci.StringError(err.Error())},
		},
	}
}

// newHandler builds a handler over the fixture with the given genesis rows,
// folding them the way the startup bootstrap does.
func newHandler(t *testing.T, f *chainFixture, balances ...gnoland.Balance) *Handler {
	t.Helper()

	vestings, err := NewVestings(balances)
	require.NoError(t, err)

	return NewHandler(
		f.client(),
		WithDenoms([]string{testDenom}),
		WithVestings(vestings),
	)
}

// ---------------------------------------------------------------
// The vesting fold, pure

func TestNewVestingsFoldsDuplicateRows(t *testing.T) {
	t.Parallel()

	// The chain applies genesis rows last-row-wins (applyBalance): two
	// vesting rows for one address keep only the last, and a plain row
	// clears a vesting one entirely.
	vestings, err := NewVestings([]gnoland.Balance{
		vestingBalance(vesterA, continuousSchedule(), 1000),
		// Same address again, different schedule: only this one counts.
		vestingBalance(vesterA, delayedSchedule(testDenom, 1000), 1000),
		// A vesting row then a plain row: the account is not vesting.
		vestingBalance(vesterB, continuousSchedule(), 1000),
		plainBalance(vesterB, 1000),
	})
	require.NoError(t, err)

	require.Len(t, vestings, 1, "only one folded entry must remain")

	// vesterA keeps the delayed schedule: at t=150 everything is locked.
	require.Equal(t, vesterA, vestings[0].address)
	require.Equal(t, int64(1000), vestings[0].account.LockedCoins(time.Unix(150, 0)).AmountOf(testDenom))
}

func TestNewVestingsRejectsUnbuildableSchedule(t *testing.T) {
	t.Parallel()

	// A schedule naming more than the row's balance cannot become a chain
	// account, so NewVestings refuses it instead of silently skipping it.
	_, err := NewVestings([]gnoland.Balance{
		{
			Address: vesterA,
			Amount:  std.NewCoins(coin(testDenom, 100)),
			Vesting: continuousSchedule(), // names 1000
		},
	})
	require.ErrorContains(t, err, "invalid vesting balance")
}

// ---------------------------------------------------------------
// The schedule math, isolated from the handler

func TestUnvestedAmount(t *testing.T) {
	t.Parallel()

	vestings, err := NewVestings([]gnoland.Balance{vestingBalance(vesterA, continuousSchedule(), 1000)})
	require.NoError(t, err)

	cases := []struct {
		name string
		at   int64
		want int64
	}{
		{"before start, all locked", 50, 1000},
		{"halfway, half locked", 150, 500},
		{"after end, none locked", 300, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			unvested := vestings[0].account.LockedCoins(time.Unix(tc.at, 0)).AmountOf(testDenom)
			require.Equal(t, tc.want, unvested)
		})
	}
}

func TestDelayedScheduleUnvested(t *testing.T) {
	t.Parallel()

	vestings, err := NewVestings([]gnoland.Balance{
		vestingBalance(vesterA, delayedSchedule(testDenom, 1000), 1000),
	})
	require.NoError(t, err)

	require.Equal(t, int64(1000), vestings[0].account.LockedCoins(time.Unix(199, 0)).AmountOf(testDenom),
		"a cliff vests nothing before the end time")
	require.Equal(t, int64(0), vestings[0].account.LockedCoins(time.Unix(200, 0)).AmountOf(testDenom),
		"a cliff vests everything at the end time")
}

// ---------------------------------------------------------------
// The snapshot

func TestRefreshBuildsConsistentSnapshot(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	fixture.supply[testDenom] = 10_000
	fixture.balances = map[crypto.Address]std.Coins{
		vesterA: std.NewCoins(coin(testDenom, 1000)), // continuous, untouched
		vesterB: std.NewCoins(coin(testDenom, 300)),  // continuous, spent into the locked portion
		vesterC: std.NewCoins(coin(testDenom, 2000)), // delayed
		plain:   std.NewCoins(coin(testDenom, 4700)), // not vesting
	}

	h := newHandler(t, fixture,
		vestingBalance(vesterA, continuousSchedule(), 1000),
		vestingBalance(vesterB, continuousSchedule(), 1000),
		vestingBalance(vesterC, delayedSchedule(testDenom, 2000), 2000),
		plainBalance(plain, 4700),
	)
	h.refresh(context.Background())

	// Every read the refresh made happened at one height, in one batch.
	require.Equal(t, 1, fixture.rec.count(), "totals and balances must share one batch")
	batch := fixture.rec.batches[0]
	require.Equal(t, int64(42), batch.height, "the batch must run at the status height")
	require.Len(t, batch.paths, 4, "one supply path plus three vesting balances")

	supply, err := h.GetSupply(context.Background(), testDenom)
	require.NoError(t, err)

	// vesterA: 1000 continuous at t=150 -> 500 locked.
	// vesterB: schedule says 500 unvested but only 300 held -> clamped to 300.
	// vesterC: delayed, end t=200, now t=150 -> all 2000 locked.
	// plain: no schedule, contributes nothing.
	require.Equal(t, &methods.Supply{
		Denom:     testDenom,
		Height:    42,
		Total:     10_000,
		Spendable: 10_000 - (500 + 300 + 2000),
		Locked:    500 + 300 + 2000,
	}, supply)
}

func TestGetSupplyFullyVested(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	fixture.status = statusAt(50, 300) // past the end time
	fixture.supply[testDenom] = 1000
	fixture.balances[vesterA] = std.NewCoins(coin(testDenom, 1000))

	h := newHandler(t, fixture, vestingBalance(vesterA, continuousSchedule(), 1000))
	h.refresh(context.Background())

	supply, err := h.GetSupply(context.Background(), testDenom)
	require.NoError(t, err)
	require.Equal(t, int64(0), supply.Locked)
	require.Equal(t, int64(1000), supply.Spendable)
}

func TestGetSupplyBeforeFirstRefresh(t *testing.T) {
	t.Parallel()

	h := newHandler(t, newChainFixture())

	_, err := h.GetSupply(context.Background(), testDenom)
	require.ErrorIs(t, err, ErrNotReady)
}

// A failed refresh must leave the previous snapshot serving.
func TestRefreshFailureKeepsLastSnapshot(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	fixture.supply[testDenom] = 100

	h := newHandler(t, fixture)
	h.refresh(context.Background())

	before, err := h.GetSupply(context.Background(), testDenom)
	require.NoError(t, err)

	fixture.mu.Lock()
	fixture.errBatch = fmt.Errorf("node unreachable")
	fixture.mu.Unlock()

	h.refresh(context.Background())

	after, err := h.GetSupply(context.Background(), testDenom)
	require.NoError(t, err)
	require.Equal(t, before, after, "a failed refresh must keep the last good snapshot")
}

// A chain without the bank/supply route must fail the refresh, not serve
// zeroes as if they were an answer.
func TestRefreshFailsOnUnsupportedSupplyRoute(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	fixture.errSupplyResponse = fmt.Errorf("unknown bank query endpoint")

	h := newHandler(t, fixture)
	h.refresh(context.Background())

	_, err := h.GetSupply(context.Background(), testDenom)
	require.ErrorIs(t, err, ErrNotReady, "no snapshot must be stored from a failed refresh")
}

// A panic inside the background refresh must not take the process down, and
// must leave the previous snapshot serving.
func TestRefreshContainsPanics(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	fixture.supply[testDenom] = 100

	h := newHandler(t, fixture)
	h.refresh(context.Background())

	before, err := h.GetSupply(context.Background(), testDenom)
	require.NoError(t, err)

	fixture.mu.Lock()
	fixture.panicBatch = true
	fixture.mu.Unlock()

	require.NotPanics(t, func() {
		h.refresh(context.Background())
	})

	after, err := h.GetSupply(context.Background(), testDenom)
	require.NoError(t, err)
	require.Equal(t, before, after, "a contained panic must keep the last good snapshot")
}

// The refresh context is derived from the lifecycle context Start runs
// under, so a shutdown cancels an in-flight refresh instead of waiting out
// the compute timeout.
func TestRefreshHonorsShutdownContext(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	fixture.supply[testDenom] = 100

	client := &mockClient{
		getStatusFn: func(ctx context.Context) (*core_types.ResultStatus, error) {
			// A cancelled lifecycle ctx must fail here, before any batch.
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			fixture.mu.Lock()
			defer fixture.mu.Unlock()

			return fixture.status, nil
		},
		batchQueryFn: func(ctx context.Context, height int64, paths []string) ([]*core_types.ResultABCIQuery, error) {
			return fixture.client().batchQueryFn(ctx, height, paths)
		},
	}

	vestings, err := NewVestings(nil)
	require.NoError(t, err)

	h := NewHandler(client, WithDenoms([]string{testDenom}), WithVestings(vestings))
	h.refresh(context.Background())

	before, err := h.GetSupply(context.Background(), testDenom)
	require.NoError(t, err)
	require.Equal(t, 1, fixture.rec.count(), "precondition: one refresh ran")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h.refresh(ctx)

	after, err := h.GetSupply(context.Background(), testDenom)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, 1, fixture.rec.count(), "a cancelled lifecycle ctx must not reach the chain")
}

// Start refreshes once before the first tick, so the endpoint answers as
// soon as the handler runs rather than after one interval.
func TestStartRefreshesImmediately(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	fixture.supply[testDenom] = 100

	h := newHandler(t, fixture)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		require.NoError(t, h.Start(ctx))
	}()

	require.Eventually(t, func() bool {
		_, err := h.GetSupply(context.Background(), testDenom)

		return err == nil
	}, time.Second, 10*time.Millisecond, "a snapshot must exist before the first tick")

	cancel()
}

// ---------------------------------------------------------------
// Request validation

func TestGetSupplyValidatesInput(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	h := newHandler(t, fixture)

	// Both surfaces call GetSupply directly, so both get the validation.
	_, err := h.GetSupply(context.Background(), "UPPER")
	require.Error(t, err)

	_, err = h.GetSupply(context.Background(), "atom")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not tracked")

	require.Equal(t, 0, fixture.rec.count(), "invalid input must not reach the chain")
}

func TestGetSupplyHandlerParamValidation(t *testing.T) {
	t.Parallel()

	h := newHandler(t, newChainFixture())

	cases := []struct {
		name   string
		params []any
	}{
		{"no denom", nil},
		{"too many params", []any{testDenom, "extra"}},
		{"denom not a string", []any{42}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, rpcErr := h.GetSupplyHandler(metadata.NewMetadata(""), tc.params)
			require.NotNil(t, rpcErr)
		})
	}
}

func TestGetSupplyHandlerServesSnapshot(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	fixture.supply[testDenom] = 100

	h := newHandler(t, fixture)
	h.refresh(context.Background())

	resp, rpcErr := h.GetSupplyHandler(metadata.NewMetadata(""), []any{testDenom})
	require.Nil(t, rpcErr)

	supply, ok := resp.(*methods.Supply)
	require.True(t, ok)
	require.Equal(t, int64(100), supply.Total)
}

// ---------------------------------------------------------------
// Wire formats

// Guard the fixture's int64 rendering: amino writes int64 as a quoted string,
// and the handler must decode it back the same way.
func TestAminoInt64RoundTrip(t *testing.T) {
	t.Parallel()

	bz := amino.MustMarshalJSON(int64(31330))
	require.Equal(t, `"31330"`, string(bz))

	var back int64

	require.NoError(t, amino.UnmarshalJSON(bz, &back))
	require.Equal(t, int64(31330), back)
}

// The JSON-RPC wire format: amounts are strings, so an amount that exceeds a
// JavaScript number's exact range survives the aggregator that reads it.
func TestSupplyAmountsMarshalAsStrings(t *testing.T) {
	t.Parallel()

	bz, err := json.Marshal(&methods.Supply{
		Denom:     testDenom,
		Height:    1337,
		Total:     10_000_000_000_000_000,
		Spendable: 7_000_000_000_000_000,
		Locked:    3_000_000_000_000_000,
	})
	require.NoError(t, err)

	require.JSONEq(t, `{
		"denom": "ugnot",
		"height": 1337,
		"total": "10000000000000000",
		"spendable": "7000000000000000",
		"locked": "3000000000000000"
	}`, string(bz))
}
