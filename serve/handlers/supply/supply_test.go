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
	bft "github.com/gnolang/gno/tm2/pkg/bft/types"
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
	getGenesisDelegate func(context.Context) (*core_types.ResultGenesis, error)
	getStatusDelegate  func(context.Context) (*core_types.ResultStatus, error)
	abciQueryDelegate  func(context.Context, string, []byte) (*core_types.ResultABCIQuery, error)
)

type mockClient struct {
	abciQueryFn  abciQueryDelegate
	getGenesisFn getGenesisDelegate
	getStatusFn  getStatusDelegate
}

func (m *mockClient) GetGenesis(ctx context.Context) (*core_types.ResultGenesis, error) {
	return m.getGenesisFn(ctx)
}

func (m *mockClient) GetStatus(ctx context.Context) (*core_types.ResultStatus, error) {
	return m.getStatusFn(ctx)
}

func (m *mockClient) ABCIQuery(ctx context.Context, path string, data []byte) (*core_types.ResultABCIQuery, error) {
	return m.abciQueryFn(ctx, path, data)
}

// abciRecorder counts ABCI queries by route.
type abciRecorder struct {
	counts map[string]int
	mu     sync.Mutex
}

func (r *abciRecorder) record(route string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.counts[route]++
}

func (r *abciRecorder) get(route string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.counts[route]
}

// routeOf collapses a query path to the part that identifies the call:
// "bank/supply/ugnot" stays whole (the denom is part of what varies),
// "bank/balances/g1..." collapses to "bank/balances".
func routeOf(path string) string {
	if len(path) > len("bank/balances/") && path[:len("bank/balances/")] == "bank/balances/" {
		return "bank/balances"
	}

	return path
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

func genesisWith(balances ...gnoland.Balance) *core_types.ResultGenesis {
	return &core_types.ResultGenesis{
		Genesis: &bft.GenesisDoc{
			AppState: gnoland.GnoGenesisState{Balances: balances},
		},
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

// abciFixture routes bank/supply/<denom> and bank/balances/<addr> queries.
type abciFixture struct {
	supply   map[string]int64
	balances map[crypto.Address]std.Coins

	// errSupply, when set, is returned for every bank/supply query.
	errSupply error
}

func (f *abciFixture) handle(_ context.Context, path string, _ []byte) (*core_types.ResultABCIQuery, error) {
	if len(path) > len("bank/supply/") && path[:len("bank/supply/")] == "bank/supply/" {
		return f.handleSupply(path)
	}

	if len(path) > len("bank/balances/") && path[:len("bank/balances/")] == "bank/balances/" {
		return f.handleBalances(path)
	}

	return nil, fmt.Errorf("unknown query path %q", path)
}

func (f *abciFixture) handleSupply(path string) (*core_types.ResultABCIQuery, error) {
	if f.errSupply != nil {
		//nolint:nilerr // the error travels in the ABCI response, as it does on the wire
		return abciError(f.errSupply), nil
	}

	denom := path[len("bank/supply/"):]

	return abciData(amino.MustMarshalJSON(f.supply[denom])), nil
}

func (f *abciFixture) handleBalances(path string) (*core_types.ResultABCIQuery, error) {
	addr, err := crypto.AddressFromBech32(path[len("bank/balances/"):])
	if err != nil {
		return nil, fmt.Errorf("bad address in fixture path %q", path)
	}

	return abciData(amino.MustMarshalJSON(f.balances[addr])), nil
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

// ---------------------------------------------------------------
// The schedule math, isolated from the handler

func TestUnvestedAmount(t *testing.T) {
	t.Parallel()

	schedule := continuousSchedule()

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

			acc, err := newVestingAccount(vesterA, schedule)
			require.NoError(t, err)

			unvested := acc.LockedCoins(time.Unix(tc.at, 0)).AmountOf(testDenom)
			require.Equal(t, tc.want, unvested)
		})
	}
}

func TestDelayedScheduleUnvested(t *testing.T) {
	t.Parallel()

	acc, err := newVestingAccount(vesterA, delayedSchedule(testDenom, 1000))
	require.NoError(t, err)

	require.Equal(t, int64(1000), acc.LockedCoins(time.Unix(199, 0)).AmountOf(testDenom),
		"a cliff vests nothing before the end time")
	require.Equal(t, int64(0), acc.LockedCoins(time.Unix(200, 0)).AmountOf(testDenom),
		"a cliff vests everything at the end time")
}

// The handler end to end, against the mock chain.
func TestGetSupplyHandler(t *testing.T) {
	t.Parallel()

	rec := &abciRecorder{counts: map[string]int{}}
	fixture := &abciFixture{
		supply: map[string]int64{testDenom: 10_000},
		balances: map[crypto.Address]std.Coins{
			vesterA: std.NewCoins(coin(testDenom, 1000)), // continuous, untouched
			vesterB: std.NewCoins(coin(testDenom, 300)),  // continuous, spent into the locked portion
			vesterC: std.NewCoins(coin(testDenom, 2000)), // delayed
			plain:   std.NewCoins(coin(testDenom, 4700)), // not vesting
		},
	}

	genesis := genesisWith(
		gnoland.Balance{
			Address: vesterA,
			Amount:  std.NewCoins(coin(testDenom, 1000)),
			Vesting: continuousSchedule(),
		},
		gnoland.Balance{
			Address: vesterB,
			Amount:  std.NewCoins(coin(testDenom, 1000)),
			Vesting: continuousSchedule(),
		},
		gnoland.Balance{
			Address: vesterC,
			Amount:  std.NewCoins(coin(testDenom, 2000)),
			Vesting: delayedSchedule(testDenom, 2000),
		},
		gnoland.Balance{
			Address: plain,
			Amount:  std.NewCoins(coin(testDenom, 4700)),
		},
	)

	mock := &mockClient{
		getGenesisFn: func(context.Context) (*core_types.ResultGenesis, error) {
			return genesis, nil
		},
		getStatusFn: func(context.Context) (*core_types.ResultStatus, error) {
			return statusAt(42, 150), nil // halfway for the continuous schedules
		},
		abciQueryFn: func(ctx context.Context, path string, data []byte) (*core_types.ResultABCIQuery, error) {
			rec.record(routeOf(path))

			return fixture.handle(ctx, path, data)
		},
	}

	h := NewHandler(mock)

	resp, rpcErr := h.GetSupplyHandler(metadata.NewMetadata(""), []any{testDenom})
	require.Nil(t, rpcErr)

	supply, ok := resp.(*methods.Supply)
	require.True(t, ok)

	// vesterA: 1000 continuous at t=150 -> 500 locked.
	// vesterB: schedule says 500 unvested but only 300 held -> clamped to 300.
	// vesterC: delayed, end t=200, now t=150 -> all 2000 locked.
	// plain: no schedule, contributes nothing.
	require.Equal(t, int64(10_000), supply.Total)
	require.Equal(t, int64(500+300+2000), supply.Locked)
	require.Equal(t, int64(10_000-2800), supply.Spendable)
	require.Equal(t, int64(42), supply.Height)
	require.Equal(t, testDenom, supply.Denom)
}

func TestGetSupplyHandlerFullyVested(t *testing.T) {
	t.Parallel()

	fixture := &abciFixture{
		supply: map[string]int64{testDenom: 1000},
		balances: map[crypto.Address]std.Coins{
			vesterA: std.NewCoins(coin(testDenom, 1000)),
		},
	}

	mock := &mockClient{
		getGenesisFn: func(context.Context) (*core_types.ResultGenesis, error) {
			return genesisWith(gnoland.Balance{
				Address: vesterA,
				Amount:  std.NewCoins(coin(testDenom, 1000)),
				Vesting: continuousSchedule(),
			}), nil
		},
		getStatusFn: func(context.Context) (*core_types.ResultStatus, error) {
			return statusAt(50, 300), nil // past the end time
		},
		abciQueryFn: fixture.handle,
	}

	resp, rpcErr := NewHandler(mock).GetSupplyHandler(metadata.NewMetadata(""), []any{testDenom})
	require.Nil(t, rpcErr)

	supply := resp.(*methods.Supply)
	require.Equal(t, int64(0), supply.Locked)
	require.Equal(t, int64(1000), supply.Spendable)
}

func TestGetSupplyHandlerCache(t *testing.T) {
	t.Parallel()

	rec := &abciRecorder{counts: map[string]int{}}
	fixture := &abciFixture{
		supply:   map[string]int64{testDenom: 100},
		balances: map[crypto.Address]std.Coins{},
	}

	mock := &mockClient{
		getGenesisFn: func(context.Context) (*core_types.ResultGenesis, error) {
			return genesisWith(), nil // no vesting accounts at all
		},
		getStatusFn: func(context.Context) (*core_types.ResultStatus, error) {
			return statusAt(1, 150), nil
		},
		abciQueryFn: func(ctx context.Context, path string, data []byte) (*core_types.ResultABCIQuery, error) {
			rec.record(routeOf(path))

			return fixture.handle(ctx, path, data)
		},
	}

	h := NewHandler(mock)

	for range 3 {
		_, rpcErr := h.GetSupplyHandler(metadata.NewMetadata(""), []any{testDenom})
		require.Nil(t, rpcErr)
	}

	require.Equal(t, 1, rec.get("bank/supply/"+testDenom),
		"the cached response must not re-query the chain within the TTL")
}

func TestGetSupplyHandlerParamValidation(t *testing.T) {
	t.Parallel()

	h := NewHandler(&mockClient{
		getGenesisFn: func(context.Context) (*core_types.ResultGenesis, error) {
			return genesisWith(), nil
		},
		getStatusFn: func(context.Context) (*core_types.ResultStatus, error) {
			return statusAt(1, 150), nil
		},
		abciQueryFn: func(context.Context, string, []byte) (*core_types.ResultABCIQuery, error) {
			t.Fatal("no query should reach the chain for an invalid request")

			return nil, nil
		},
	})

	cases := []struct {
		name   string
		params []any
	}{
		{"no denom", nil},
		{"too many params", []any{testDenom, "extra"}},
		{"denom not a string", []any{42}},
		{"malformed denom", []any{"UPPER"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, rpcErr := h.GetSupplyHandler(metadata.NewMetadata(""), tc.params)
			require.NotNil(t, rpcErr)
		})
	}
}

func TestGetSupplyHandlerChainErrors(t *testing.T) {
	t.Parallel()

	t.Run("supply query not supported by the chain", func(t *testing.T) {
		t.Parallel()

		fixture := &abciFixture{errSupply: fmt.Errorf("unknown bank query endpoint")}
		h := NewHandler(&mockClient{
			getGenesisFn: func(context.Context) (*core_types.ResultGenesis, error) {
				return genesisWith(), nil
			},
			getStatusFn: func(context.Context) (*core_types.ResultStatus, error) {
				return statusAt(1, 150), nil
			},
			abciQueryFn: fixture.handle,
		})

		_, rpcErr := h.GetSupplyHandler(metadata.NewMetadata(""), []any{testDenom})
		require.NotNil(t, rpcErr)
		require.Contains(t, rpcErr.Message, "bank/supply")
	})

	t.Run("non-gno genesis state", func(t *testing.T) {
		t.Parallel()

		h := NewHandler(&mockClient{
			getGenesisFn: func(context.Context) (*core_types.ResultGenesis, error) {
				return &core_types.ResultGenesis{
					Genesis: &bft.GenesisDoc{AppState: "not-a-gno-state"},
				}, nil
			},
			getStatusFn: func(context.Context) (*core_types.ResultStatus, error) {
				return statusAt(1, 150), nil
			},
			abciQueryFn: func(context.Context, string, []byte) (*core_types.ResultABCIQuery, error) {
				t.Fatal("the handler must fail before querying balances")

				return nil, nil
			},
		})

		_, rpcErr := h.GetSupplyHandler(metadata.NewMetadata(""), []any{testDenom})
		require.NotNil(t, rpcErr)
	})
}

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
