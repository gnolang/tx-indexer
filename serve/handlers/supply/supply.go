package supply

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/crypto"
	"github.com/gnolang/gno/tm2/pkg/overflow"
	"github.com/gnolang/gno/tm2/pkg/std"
	"go.uber.org/zap"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	"github.com/gnolang/gno/tm2/pkg/amino"

	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/methods"
	"github.com/gnolang/tx-indexer/serve/spec"
)

const (
	// defaultRefreshInterval is how often the snapshot is rebuilt from the
	// chain. The indexer talks to the node on its own schedule, like the
	// fetcher — never on request.
	defaultRefreshInterval = 10 * time.Second

	// defaultComputeTimeout bounds one snapshot rebuild, so a stuck node
	// cannot stall the refresh loop.
	defaultComputeTimeout = 30 * time.Second
)

// ErrNotReady is returned before the first snapshot has been computed.
var ErrNotReady = errors.New("supply snapshot not ready yet")

// Client is the chain access the supply handler needs. Small on purpose:
// the handler must stay testable against a mock, and the fetcher's Client
// carries fetching concerns this handler does not have.
type Client interface {
	// GetStatus returns the chain status, for the height and block time
	// the snapshot is computed at
	GetStatus(context.Context) (*core_types.ResultStatus, error)

	// ABCIQueryBatchAtHeight runs every path as an ABCI query at exactly
	// height, in one batch. Results are returned in path order.
	ABCIQueryBatchAtHeight(ctx context.Context, height int64, paths []string) ([]*core_types.ResultABCIQuery, error)
}

// Vesting is one genesis vesting account, with the schedule pre-built
// into an account so the locked math is the chain's own.
type Vesting struct {
	account std.VestingAccount
	address crypto.Address
}

// NewVestings folds the genesis balance rows per address — last row wins,
// and a plain row clears a previous vesting one, mirroring the chain's
// applyBalance — and builds the chain accounts for the surviving schedules,
// so a duplicated or overridden genesis row cannot double-count a lock.
//
// Pure: no network, no clock. The genesis balances come from the startup
// bootstrap (fetch.BootstrapGenesis), so a failure here is a startup
// failure, not something a background loop has to contain.
func NewVestings(balances []gnoland.Balance) ([]Vesting, error) {
	schedules := make(map[crypto.Address]gnoland.Balance)

	for _, balance := range balances {
		if !balance.IsVesting() {
			// A plain row replaces any vesting row before it, exactly as
			// the chain replaces the account.
			delete(schedules, balance.Address)

			continue
		}

		schedules[balance.Address] = balance
	}

	vestings := make([]Vesting, 0, len(schedules))

	for address, balance := range schedules {
		account, err := newVestingAccount(address, balance.Amount, balance.Vesting)
		if err != nil {
			// Fail rather than skip: a schedule the chain's own types
			// cannot rebuild would silently undercount locked.
			return nil, fmt.Errorf("invalid vesting balance for %s: %w", address, err)
		}

		vestings = append(vestings, Vesting{
			account: account,
			address: address,
		})
	}

	return vestings, nil
}

// newVestingAccount rebuilds the chain's account for a genesis schedule,
// funded with the row's amount so the chain constructor validates the
// schedule against it exactly as InitChain does.
func newVestingAccount(
	addr crypto.Address,
	coins std.Coins,
	schedule *std.VestingSchedule,
) (std.VestingAccount, error) {
	base := std.NewBaseAccount(addr, coins, nil, 0, 0)

	switch schedule.Type {
	case std.VestingDelayed:
		return std.NewDelayedVestingAccount(base, *schedule)
	default:
		return std.NewContinuousVestingAccount(base, *schedule)
	}
}

// snapshot is one consistent view of the tracked supplies. Every figure in
// it was read at the same chain height.
type snapshot struct {
	supplies map[string]*methods.Supply
}

// Handler serves the supply of pre-registered denoms from a background
// snapshot, refreshed on the handler's own schedule. Requests never reach
// the chain: the denom set is fixed at construction, so request input cannot
// amplify into node load, and a snapshot is either served whole or the last
// good one keeps serving while a refresh fails.
type Handler struct {
	client          Client
	logger          *zap.Logger
	snapshot        atomic.Pointer[snapshot]
	denoms          []string
	vestings        []Vesting
	refreshInterval time.Duration
	computeTimeout  time.Duration
}

// Option is a functional option for the supply Handler.
type Option func(*Handler)

// WithLogger sets the handler logger.
func WithLogger(logger *zap.Logger) Option {
	return func(h *Handler) {
		h.logger = logger
	}
}

// WithDenoms sets the tracked denominations.
func WithDenoms(denoms []string) Option {
	return func(h *Handler) {
		h.denoms = denoms
	}
}

// WithVestings sets the genesis vesting accounts, from NewVestings over the
// bootstrap's genesis balances.
func WithVestings(vestings []Vesting) Option {
	return func(h *Handler) {
		h.vestings = vestings
	}
}

// WithRefreshInterval overrides how often the snapshot is rebuilt.
func WithRefreshInterval(interval time.Duration) Option {
	return func(h *Handler) {
		h.refreshInterval = interval
	}
}

// WithComputeTimeout overrides the bound on one snapshot rebuild.
func WithComputeTimeout(timeout time.Duration) Option {
	return func(h *Handler) {
		h.computeTimeout = timeout
	}
}

func NewHandler(client Client, opts ...Option) *Handler {
	h := &Handler{
		client:          client,
		logger:          zap.NewNop(),
		refreshInterval: defaultRefreshInterval,
		computeTimeout:  defaultComputeTimeout,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// Start runs the handler's own schedule, refreshing the snapshot on a
// ticker. Blocks until ctx is done.
func (h *Handler) Start(ctx context.Context) error {
	ticker := time.NewTicker(h.refreshInterval)
	defer ticker.Stop()

	for {
		// The refresh also runs before the first tick, so the endpoint
		// answers as soon as it can rather than after one interval.
		h.refresh(ctx)

		select {
		case <-ctx.Done():
			h.logger.Info("Supply handler shut down")

			return nil
		case <-ticker.C:
		}
	}
}

// GetSupply returns the supply of denom split into total, spendable and
// locked, from the latest snapshot. It touches no network: both the GraphQL
// and JSON-RPC surfaces call straight through here, so the validation lives
// here too.
func (h *Handler) GetSupply(_ context.Context, denom string) (*methods.Supply, error) {
	if err := std.ValidateDenom(denom); err != nil {
		return nil, fmt.Errorf("invalid denom %q: %w", denom, err)
	}

	if !h.tracked(denom) {
		return nil, fmt.Errorf("denom %q is not tracked (tracked: %v)", denom, h.denoms)
	}

	snap := h.snapshot.Load()
	if snap == nil {
		return nil, ErrNotReady
	}

	supply, ok := snap.supplies[denom]
	if !ok {
		// Unreachable while snapshots are built from the tracked list; a
		// distinct label so a bug here is not misread as an input problem.
		return nil, fmt.Errorf("denom %q missing from the snapshot (tracked: %v)", denom, h.denoms)
	}

	return supply, nil
}

// GetSupplyHandler returns the supply of a single denomination, split into
// total, spendable and locked, from the latest snapshot.
//
// params: [denom]
func (h *Handler) GetSupplyHandler(
	_ *metadata.Metadata,
	params []any,
) (any, *spec.BaseJSONError) {
	if len(params) != 1 {
		return nil, spec.GenerateInvalidParamCountError()
	}

	denom, ok := params[0].(string)
	if !ok {
		return nil, spec.GenerateInvalidParamError(1)
	}

	supply, err := h.GetSupply(context.Background(), denom)
	if err != nil {
		return nil, spec.GenerateResponseError(err)
	}

	return supply, nil
}

func (h *Handler) tracked(denom string) bool {
	for _, d := range h.denoms {
		if d == denom {
			return true
		}
	}

	return false
}

// refresh rebuilds the snapshot. Every read happens at one chain height, in
// one batched round trip. The context is the lifecycle one Start runs under,
// bounded by a timeout — so a shutdown cancels an in-flight refresh instead
// of waiting out the timeout, while no request is ever waited on or able to
// fail a refresh. A failed refresh leaves the previous snapshot serving, and
// a panic is contained: this is a background loop with no request to unwind
// into, and it must not take the indexer down.
func (h *Handler) refresh(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			h.logger.Error(
				"panic refreshing supply snapshot, serving the previous one",
				zap.Any("panic", r),
				zap.Stack("stack"),
			)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, h.computeTimeout)
	defer cancel()

	snap, err := h.computeSnapshot(ctx)
	if err != nil {
		h.logger.Warn("unable to refresh supply snapshot, serving the previous one", zap.Error(err))

		return
	}

	h.snapshot.Store(snap)
}

func (h *Handler) computeSnapshot(ctx context.Context) (*snapshot, error) {
	status, err := h.client.GetStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get chain status, %w", err)
	}

	height := status.SyncInfo.LatestBlockHeight
	blockTime := status.SyncInfo.LatestBlockTime

	// One batch, one height: the totals and every vesting balance come from
	// the same block, and the round trips are the handler's own, not a
	// request's.
	paths := make([]string, 0, len(h.denoms)+len(h.vestings))
	for _, denom := range h.denoms {
		paths = append(paths, "bank/supply/"+denom)
	}

	for _, vesting := range h.vestings {
		paths = append(paths, "bank/balances/"+crypto.AddressToBech32(vesting.address))
	}

	results, err := h.client.ABCIQueryBatchAtHeight(ctx, height, paths)
	if err != nil {
		return nil, fmt.Errorf("unable to query the chain at height %d, %w", height, err)
	}

	if len(results) != len(paths) {
		return nil, fmt.Errorf("expected %d query results at height %d, got %d", len(paths), height, len(results))
	}

	totals := make(map[string]int64, len(h.denoms))

	for i, denom := range h.denoms {
		total, err := decodeSupply(results[i])
		if err != nil {
			return nil, fmt.Errorf("bank/supply/%s at height %d: %w", denom, height, err)
		}

		totals[denom] = total
	}

	balances := make(map[crypto.Address]std.Coins, len(h.vestings))

	for i, vesting := range h.vestings {
		coins, err := decodeBalance(results[len(h.denoms)+i])
		if err != nil {
			return nil, fmt.Errorf("bank/balances/%s at height %d: %w", vesting.address, height, err)
		}

		balances[vesting.address] = coins
	}

	supplies := make(map[string]*methods.Supply, len(h.denoms))

	for _, denom := range h.denoms {
		// Locked is the still-unvested amount clamped to the balance
		// actually held — fees and storage refunds bypass the lock and can
		// spend into the locked portion, and a lock on coins nobody holds
		// locks nothing.
		var locked int64

		for _, vesting := range h.vestings {
			unvested := vesting.account.LockedCoins(blockTime).AmountOf(denom)
			if unvested <= 0 {
				continue
			}

			amount := min(balances[vesting.address].AmountOf(denom), unvested)

			sum, ok := overflow.Add(locked, amount)
			if !ok {
				sum = locked // skip the unrepresentable remainder rather than go negative
			}

			locked = sum
		}

		total := totals[denom]

		spendable := total - locked
		if spendable < 0 {
			// The counter disagrees with the balances; report the floor
			// rather than a negative circulating supply.
			spendable = 0
		}

		supplies[denom] = &methods.Supply{
			Denom:     denom,
			Height:    height,
			Total:     total,
			Spendable: spendable,
			Locked:    locked,
		}
	}

	return &snapshot{supplies: supplies}, nil
}

func decodeSupply(res *core_types.ResultABCIQuery) (int64, error) {
	if err := responseError(res); err != nil {
		return 0, err
	}

	var total int64

	if err := amino.UnmarshalJSON(res.Response.Data, &total); err != nil {
		return 0, fmt.Errorf("unable to decode supply: %w", err)
	}

	return total, nil
}

func decodeBalance(res *core_types.ResultABCIQuery) (std.Coins, error) {
	if err := responseError(res); err != nil {
		return nil, err
	}

	var coins std.Coins

	if err := amino.UnmarshalJSON(res.Response.Data, &coins); err != nil {
		return nil, fmt.Errorf("unable to decode balance: %w", err)
	}

	return coins, nil
}

func responseError(res *core_types.ResultABCIQuery) error {
	if res == nil {
		return errors.New("nil query result")
	}

	// A chain without the bank/supply route reports it here; the height in
	// the message names the state the refresh was reading.
	if res.Response.Error != nil {
		return fmt.Errorf("query error: %w", res.Response.Error)
	}

	return nil
}
