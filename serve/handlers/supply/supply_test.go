package supply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

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

// mockStorage serves the genesis balance rows the startup bootstrap would have
// written. errBalances, when set, fails the read.
type mockStorage struct {
	errBalances error
	balances    []gnoland.Balance
}

func (m *mockStorage) GetGenesisBalances() ([]gnoland.Balance, error) {
	if m.errBalances != nil {
		return nil, m.errBalances
	}

	return m.balances, nil
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

// coin is amount of the test denom.
func coin(amount int64) std.Coin {
	return std.NewCoin(testDenom, amount)
}

// continuousSchedule vests 1000 of the test denom linearly from t=100 to t=200.
func continuousSchedule() *std.VestingSchedule {
	return &std.VestingSchedule{
		OriginalVesting: std.NewCoins(coin(1000)),
		StartTime:       100,
		EndTime:         200,
	}
}

// delayedSchedule vests amount of the test denom fully at t=200 (cliff).
func delayedSchedule(amount int64) *std.VestingSchedule {
	return &std.VestingSchedule{
		OriginalVesting: std.NewCoins(coin(amount)),
		StartTime:       0,
		EndTime:         200,
		Type:            std.VestingDelayed,
	}
}

// vestingBalance is a genesis row that vests amount of the test denom.
func vestingBalance(addr crypto.Address, schedule *std.VestingSchedule, amount int64) gnoland.Balance {
	return gnoland.Balance{
		Address: addr,
		Amount:  std.NewCoins(coin(amount)),
		Vesting: schedule,
	}
}

// plainBalance is a genesis row without a vesting schedule.
func plainBalance(addr crypto.Address, amount int64) gnoland.Balance {
	return gnoland.Balance{
		Address: addr,
		Amount:  std.NewCoins(coin(amount)),
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

// chainFixture answers one batch at one height, from a plain map.
// errBatch fails the whole batch when set; errSupplyResponse makes every
// bank/supply result carry an ABCI error (a chain without the route);
// panicBatch makes the batch call panic. Only bank/supply is answered: the
// refresh must never ask for a per-account balance.
type chainFixture struct {
	supply map[string]int64
	rec    *batchRecorder
	status *core_types.ResultStatus

	errBatch          error
	errSupplyResponse error

	mu         sync.Mutex
	panicBatch bool
}

func newChainFixture() *chainFixture {
	return &chainFixture{
		supply: map[string]int64{},
		rec:    &batchRecorder{},
		status: statusAt(42, 150),
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

// newHandler builds a handler over the fixture, with the given genesis rows
// waiting in the storage the way the startup bootstrap leaves them.
func newHandler(t *testing.T, f *chainFixture, balances ...gnoland.Balance) *Handler {
	t.Helper()

	h := NewHandler(
		f.client(),
		&mockStorage{balances: balances},
		WithDenoms([]string{testDenom}),
	)

	// Start loads the schedules before serving; these tests drive refresh
	// directly, so they do the same.
	require.NoError(t, h.loadWindows())

	return h
}

// ---------------------------------------------------------------
// The vesting fold, pure

func TestFoldVestingWindowsFoldsDuplicateRows(t *testing.T) {
	t.Parallel()

	// The chain applies genesis rows last-row-wins (applyBalance): two
	// vesting rows for one address keep only the last, and a plain row
	// clears a vesting one entirely.
	windows, err := FoldVestingWindows([]gnoland.Balance{
		vestingBalance(vesterA, continuousSchedule(), 1000),
		// Same address again, different schedule: only this one counts.
		vestingBalance(vesterA, delayedSchedule(1000), 1000),
		// A vesting row then a plain row: the account is not vesting.
		vestingBalance(vesterB, continuousSchedule(), 1000),
		plainBalance(vesterB, 1000),
	})
	require.NoError(t, err)

	require.Len(t, windows, 1, "only one folded entry must remain")

	// vesterA keeps the delayed schedule: at t=150 everything is locked.
	require.Equal(t, std.VestingDelayed, windows[0].Type)
	require.Equal(t, int64(1000), windows[0].LockedCoins(time.Unix(150, 0)).AmountOf(testDenom))
}

func TestFoldVestingWindowsSumsSchedulesSharingAWindow(t *testing.T) {
	t.Parallel()

	// Rows vesting over the same window vest identically, so they fold into
	// one schedule holding the summed amount, whatever the number of
	// accounts behind it. A different curve over the same bounds stays its
	// own schedule.
	windows, err := FoldVestingWindows([]gnoland.Balance{
		vestingBalance(vesterA, continuousSchedule(), 1000),
		vestingBalance(vesterB, &std.VestingSchedule{
			OriginalVesting: std.NewCoins(coin(500)),
			StartTime:       100,
			EndTime:         200,
		}, 500),
		vestingBalance(vesterC, delayedSchedule(2000), 2000),
	})
	require.NoError(t, err)
	require.Len(t, windows, 2, "one continuous window and one cliff")

	byType := make(map[std.VestingScheduleType]std.VestingSchedule, len(windows))
	for _, window := range windows {
		byType[window.Type] = window
	}

	require.Equal(t, int64(1500), byType[std.VestingContinuous].OriginalVesting.AmountOf(testDenom))
	require.Equal(t, int64(2000), byType[std.VestingDelayed].OriginalVesting.AmountOf(testDenom))

	// Halfway through the window, half of the summed amount is still locked.
	require.Equal(t, int64(750), byType[std.VestingContinuous].LockedCoins(time.Unix(150, 0)).AmountOf(testDenom))
}

func TestFoldVestingWindowsRejectsUnderfundedSchedule(t *testing.T) {
	t.Parallel()

	// A schedule naming more than the row's balance is a row the chain
	// rejects at InitChain, so FoldVestingWindows refuses it instead of silently
	// skipping it.
	_, err := FoldVestingWindows([]gnoland.Balance{
		{
			Address: vesterA,
			Amount:  std.NewCoins(coin(100)),
			Vesting: continuousSchedule(), // names 1000
		},
	})
	require.ErrorContains(t, err, "invalid vesting balance")
}

func TestFoldVestingWindowsRejectsMalformedSchedule(t *testing.T) {
	t.Parallel()

	// A continuous schedule ending before it starts is a row the chain
	// rejects at InitChain, so the fold refuses it rather than locking an
	// amount no account vests.
	_, err := FoldVestingWindows([]gnoland.Balance{
		vestingBalance(vesterA, &std.VestingSchedule{
			OriginalVesting: std.NewCoins(coin(1000)),
			StartTime:       200,
			EndTime:         100,
		}, 1000),
	})
	require.ErrorContains(t, err, "invalid vesting balance")
}

func TestFoldVestingWindowsRejectsOverflowingWindow(t *testing.T) {
	t.Parallel()

	// Each row is valid on its own; together they exceed what an int64
	// holds. The fold must report that, not panic the process at startup.
	huge := int64(1) << 62

	rows := make([]gnoland.Balance, 0, 3)
	for _, addr := range []crypto.Address{vesterA, vesterB, vesterC} {
		rows = append(rows, vestingBalance(addr, &std.VestingSchedule{
			OriginalVesting: std.NewCoins(coin(huge)),
			StartTime:       100,
			EndTime:         200,
		}, huge))
	}

	_, err := FoldVestingWindows(rows)
	require.ErrorContains(t, err, "overflow")
}

// A cliff vests nothing until its end whatever its start, so delayed rows
// differing only in start time are one window.
func TestFoldVestingWindowsIgnoresCliffStartTime(t *testing.T) {
	t.Parallel()

	later := delayedSchedule(1000)
	later.StartTime = 50

	windows, err := FoldVestingWindows([]gnoland.Balance{
		vestingBalance(vesterA, delayedSchedule(1000), 1000),
		vestingBalance(vesterB, later, 1000),
	})
	require.NoError(t, err)
	require.Len(t, windows, 1)
	require.Equal(t, int64(2000), windows[0].OriginalVesting.AmountOf(testDenom))
}

// The summed window vests at least what the accounts vest one by one, and at
// most one unit per denom more per account: the chain rounds each account's
// vested amount down, and the sum rounds down once.
func TestFoldVestingWindowsRoundsDownOncePerWindow(t *testing.T) {
	t.Parallel()

	amounts := []int64{1, 3, 7}
	at := time.Unix(137, 0) // 37% through the window: nothing divides evenly

	rows := make([]gnoland.Balance, 0, len(amounts))

	var perAccount int64

	for i, amount := range amounts {
		schedule := &std.VestingSchedule{
			OriginalVesting: std.NewCoins(coin(amount)),
			StartTime:       100,
			EndTime:         200,
		}
		addr := crypto.AddressFromPreimage(fmt.Appendf(nil, "rounding-%d", i))

		rows = append(rows, vestingBalance(addr, schedule, amount))
		perAccount += schedule.LockedCoins(at).AmountOf(testDenom)
	}

	windows, err := FoldVestingWindows(rows)
	require.NoError(t, err)
	require.Len(t, windows, 1)

	summed := windows[0].LockedCoins(at).AmountOf(testDenom)
	require.LessOrEqual(t, summed, perAccount, "the summed window must not lock more than the accounts do")
	require.Greater(t, summed, perAccount-int64(len(amounts)), "the shortfall stays below one unit per account")
}

// Windows come out in a fixed order whatever the map iteration order, so
// snapshots and logs are reproducible: by end time, then start time, then
// curve.
func TestFoldVestingWindowsOrdersWindows(t *testing.T) {
	t.Parallel()

	windows, err := FoldVestingWindows([]gnoland.Balance{
		vestingBalance(vesterA, delayedSchedule(1000), 1000), // cliff at 200
		vestingBalance(vesterB, continuousSchedule(), 1000),  // 100 to 200
		vestingBalance(vesterC, &std.VestingSchedule{ // 0 to 150
			OriginalVesting: std.NewCoins(coin(1000)),
			StartTime:       0,
			EndTime:         150,
		}, 1000),
	})
	require.NoError(t, err)
	require.Len(t, windows, 3)

	require.Equal(t, int64(150), windows[0].EndTime)
	require.Equal(t, std.VestingDelayed, windows[1].Type,
		"a cliff has no start, so it sorts before a window starting later")
	require.Equal(t, std.VestingContinuous, windows[2].Type)
}

// ---------------------------------------------------------------
// The schedule math, isolated from the handler

func TestUnvestedAmount(t *testing.T) {
	t.Parallel()

	windows, err := FoldVestingWindows([]gnoland.Balance{vestingBalance(vesterA, continuousSchedule(), 1000)})
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

			unvested := windows[0].LockedCoins(time.Unix(tc.at, 0)).AmountOf(testDenom)
			require.Equal(t, tc.want, unvested)
		})
	}
}

func TestDelayedScheduleUnvested(t *testing.T) {
	t.Parallel()

	windows, err := FoldVestingWindows([]gnoland.Balance{
		vestingBalance(vesterA, delayedSchedule(1000), 1000),
	})
	require.NoError(t, err)

	require.Equal(t, int64(1000), windows[0].LockedCoins(time.Unix(199, 0)).AmountOf(testDenom),
		"a cliff vests nothing before the end time")
	require.Equal(t, int64(0), windows[0].LockedCoins(time.Unix(200, 0)).AmountOf(testDenom),
		"a cliff vests everything at the end time")
}

// ---------------------------------------------------------------
// The snapshot

func TestRefreshBuildsConsistentSnapshot(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	fixture.supply[testDenom] = 10_000

	h := newHandler(t, fixture,
		vestingBalance(vesterA, continuousSchedule(), 1000),
		vestingBalance(vesterB, continuousSchedule(), 1000),
		vestingBalance(vesterC, delayedSchedule(2000), 2000),
		plainBalance(plain, 4700),
	)
	h.refresh(context.Background())

	// Every read the refresh made happened at one height, in one batch,
	// and only the supply counter was read: never a per-account balance.
	require.Equal(t, 1, fixture.rec.count(), "totals must come from one batch")
	batch := fixture.rec.batches[0]
	require.Equal(t, int64(42), batch.height, "the batch must run at the status height")
	require.Equal(t, []string{"bank/supply/" + testDenom}, batch.paths, "only the supply path, never a balance")

	supply, err := h.GetSupply(context.Background(), testDenom)
	require.NoError(t, err)

	// vesterA: 1000 continuous at t=150 -> 500 locked.
	// vesterB: 1000 continuous at t=150 -> 500 locked, from the schedule
	//          alone: the live balance is never read, so nothing clamps it.
	// vesterC: delayed, end t=200, now t=150 -> all 2000 locked.
	// plain: no schedule, contributes nothing.
	require.Equal(t, &methods.Supply{
		Denom:     testDenom,
		Height:    42,
		Total:     10_000,
		Spendable: 10_000 - (500 + 500 + 2000),
		Locked:    500 + 500 + 2000,
	}, supply)
}

func TestGetSupplyFullyVested(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	fixture.status = statusAt(50, 300) // past the end time
	fixture.supply[testDenom] = 1000

	h := newHandler(t, fixture, vestingBalance(vesterA, continuousSchedule(), 1000))
	h.refresh(context.Background())

	supply, err := h.GetSupply(context.Background(), testDenom)
	require.NoError(t, err)
	require.Equal(t, int64(0), supply.Locked)
	require.Equal(t, int64(1000), supply.Spendable)
}

// The supply counter is the ceiling for locked: schedules can name more than
// the counter holds once vesting accounts spent coins through fees or
// deposits, and the response must keep total = spendable + locked. The
// disagreement is a data problem, so it is logged when it appears and when it
// clears, not on every refresh in between.
func TestRefreshClampsLockedToTotal(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	fixture.supply[testDenom] = 100 // the schedule below still locks 1000 at t=150

	core, logs := observer.New(zapcore.InfoLevel)

	h := NewHandler(
		fixture.client(),
		&mockStorage{balances: []gnoland.Balance{
			vestingBalance(vesterA, delayedSchedule(1000), 1000),
		}},
		WithDenoms([]string{testDenom}),
		WithLogger(zap.New(core)),
	)
	require.NoError(t, h.loadWindows())

	h.refresh(context.Background())
	h.refresh(context.Background())

	supply, err := h.GetSupply(context.Background(), testDenom)
	require.NoError(t, err)
	require.Equal(t, int64(100), supply.Total)
	require.Equal(t, int64(100), supply.Locked, "locked must not exceed the supply counter")
	require.Equal(t, int64(0), supply.Spendable)

	const clampedMsg = "locked exceeds the supply counter, clamping"

	require.Len(t, logs.FilterMessage(clampedMsg).All(), 1, "the disagreement is logged once, when it appears")

	fixture.mu.Lock()
	fixture.supply[testDenom] = 5000
	fixture.mu.Unlock()

	h.refresh(context.Background())

	supply, err = h.GetSupply(context.Background(), testDenom)
	require.NoError(t, err)
	require.Equal(t, int64(1000), supply.Locked)
	require.Len(t, logs.FilterMessage(clampedMsg).All(), 1)
	require.Len(t, logs.FilterMessage("locked is within the supply counter again").All(), 1,
		"the recovery is logged once")
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

// A negative supply counter is a malformed answer, not a supply. The refresh
// must fail rather than publish a negative locked figure.
func TestRefreshFailsOnNegativeSupply(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	fixture.supply[testDenom] = -1

	h := newHandler(t, fixture)
	h.refresh(context.Background())

	_, err := h.GetSupply(context.Background(), testDenom)
	require.ErrorIs(t, err, ErrNotReady, "no snapshot must be stored from a negative counter")
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

	h := NewHandler(client, &mockStorage{}, WithDenoms([]string{testDenom}))
	require.NoError(t, h.loadWindows())
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

// The bootstrap commits the balances before any service starts, so a storage
// that cannot serve them is corrupt. Start fails rather than serving a supply
// with no vesting schedules, which would report everything as circulating.
func TestStartFailsOnUnreadableGenesisBalances(t *testing.T) {
	t.Parallel()

	fixture := newChainFixture()
	readErr := errors.New("genesis balances missing")

	h := NewHandler(
		fixture.client(),
		&mockStorage{errBalances: readErr},
		WithDenoms([]string{testDenom}),
	)

	require.ErrorIs(t, h.Start(context.Background()), readErr)
	require.Zero(t, fixture.rec.count(), "no refresh must run without the schedules")
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
