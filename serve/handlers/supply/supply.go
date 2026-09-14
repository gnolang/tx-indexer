package supply

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	"github.com/gnolang/gno/tm2/pkg/amino"
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/crypto"
	"github.com/gnolang/gno/tm2/pkg/overflow"
	"github.com/gnolang/gno/tm2/pkg/std"
	"go.uber.org/zap"

	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/methods"
	"github.com/gnolang/tx-indexer/serve/spec"
)

const (
	// defaultRefreshInterval is how often the snapshot is rebuilt from
	// the chain. Like the fetcher, the handler talks to the node on its
	// own schedule and never on request.
	defaultRefreshInterval = 10 * time.Second

	// defaultComputeTimeout bounds one snapshot rebuild, so a stuck node
	// cannot stall the refresh loop.
	defaultComputeTimeout = 30 * time.Second
)

// ErrNotReady is returned before the first snapshot has been computed.
var ErrNotReady = errors.New("supply snapshot not ready yet")

// Client is the chain access the supply handler needs. Kept small so the
// handler can be tested against a mock; the fetcher's Client drags in
// fetching concerns this handler does not care about.
type Client interface {
	// GetStatus returns the chain status, for the height and block time
	// the snapshot is computed at
	GetStatus(context.Context) (*core_types.ResultStatus, error)

	// ABCIQueryBatchAtHeight runs every path as an ABCI query at exactly
	// height, in one batch. Results are returned in path order.
	ABCIQueryBatchAtHeight(ctx context.Context, height int64, paths []string) ([]*core_types.ResultABCIQuery, error)
}

// Storage is the storage access the supply handler needs. The genesis balance
// rows are put there by the startup bootstrap, so the handler reads its own
// input instead of having it threaded down from main.
type Storage interface {
	// GetGenesisBalances returns the stored genesis balance rows, unfolded
	GetGenesisBalances() ([]gnoland.Balance, error)
}

// vestingWindow identifies schedules that vest identically: the same curve
// over the same bounds. Amounts under one window can be summed and vested as
// one schedule.
type vestingWindow struct {
	Type      std.VestingScheduleType
	StartTime int64
	EndTime   int64
}

// FoldVestingWindows folds the genesis balance rows into one schedule per
// vesting window, which is all the locked math needs. Rows fold per address
// with the same rule the chain's applyBalance uses: the last row for an
// address wins, and a plain row clears an earlier vesting one, so a duplicated
// or overridden row cannot double-count a lock. The surviving rows are
// verified, then summed per window: rows vesting over the same window vest
// identically, so one schedule holding their total locks what the rows would
// lock one by one, and the handler's cost follows the number of distinct
// windows in the genesis rather than the number of accounts. A continuous
// window's summed schedule can vest up to one unit per denom more per account
// than the accounts do separately, because the chain rounds each account's
// vested amount down. A window's total per denom must fit in an int64, as the
// chain's own supply counter must; rows that push it past that are rejected.
//
// Pure: no network, no clock. The rows come from the storage, where the
// startup bootstrap put them, so they are folded fresh on every boot — a fix
// to the fold reaches an existing database without refetching genesis.
func FoldVestingWindows(balances []gnoland.Balance) ([]std.VestingSchedule, error) {
	// Sized for a genesis where every row vests, the shape of gnoland-1.
	rows := make(map[crypto.Address]gnoland.Balance, len(balances))

	for _, balance := range balances {
		if !balance.IsVesting() {
			// A plain row replaces any vesting row before it, exactly as
			// the chain replaces the account.
			delete(rows, balance.Address)

			continue
		}

		rows[balance.Address] = balance
	}

	// Per window, the summed amount of each denom. Summed as int64 with an
	// overflow check rather than through std.Coins.Add, which panics on
	// overflow: a genesis the fold cannot represent is an error to report,
	// not a crash at startup.
	amounts := make(map[vestingWindow]map[string]int64)

	for address, balance := range rows {
		// Verify rejects a malformed schedule and a schedule the row's amount
		// does not cover, the same rejections the chain makes at InitChain.
		// Fail rather than skip: a skipped row silently undercounts locked.
		if err := balance.Verify(); err != nil {
			return nil, fmt.Errorf("invalid vesting balance for %s: %w", address, err)
		}

		window := windowOf(*balance.Vesting)

		byDenom, seen := amounts[window]
		if !seen {
			byDenom = make(map[string]int64)
			amounts[window] = byDenom
		}

		for _, coin := range balance.Vesting.OriginalVesting {
			sum, ok := overflow.Add(byDenom[coin.Denom], coin.Amount)
			if !ok {
				return nil, fmt.Errorf(
					"vesting amount of %s overflows int64 in the window ending at %d",
					coin.Denom,
					window.EndTime,
				)
			}

			byDenom[coin.Denom] = sum
		}
	}

	windows := make([]std.VestingSchedule, 0, len(amounts))

	for window, byDenom := range amounts {
		coins := make([]std.Coin, 0, len(byDenom))
		for denom, amount := range byDenom {
			coins = append(coins, std.NewCoin(denom, amount))
		}

		windows = append(windows, std.VestingSchedule{
			// NewCoins sorts by denom, the order the schedule math relies on.
			OriginalVesting: std.NewCoins(coins...),
			StartTime:       window.StartTime,
			EndTime:         window.EndTime,
			Type:            window.Type,
		})
	}

	// Map order is random; a fixed order keeps snapshots and logs reproducible.
	slices.SortFunc(windows, func(a, b std.VestingSchedule) int {
		return cmp.Or(
			cmp.Compare(a.EndTime, b.EndTime),
			cmp.Compare(a.StartTime, b.StartTime),
			cmp.Compare(a.Type, b.Type),
		)
	})

	return windows, nil
}

// windowOf is the vesting window of a schedule. A cliff has no start: the
// chain ignores StartTime for delayed schedules, so it is left out of the
// window rather than splitting identical cliffs over an unused field.
func windowOf(schedule std.VestingSchedule) vestingWindow {
	window := vestingWindow{
		Type:      schedule.Type,
		StartTime: schedule.StartTime,
		EndTime:   schedule.EndTime,
	}

	if window.Type == std.VestingDelayed {
		window.StartTime = 0
	}

	return window
}

// snapshot is one consistent view of the tracked supplies. Every figure in
// it was read at the same chain height.
type snapshot struct {
	supplies map[string]*methods.Supply
	// clamped holds the denoms whose locked figure was capped at the supply
	// counter, so the next refresh logs only a change in that state.
	clamped map[string]bool
}

// Handler serves the supply of pre-registered denoms from a background
// snapshot that it refreshes on its own schedule. Requests never touch
// the chain. The denom set is fixed at construction, so request input
// cannot turn into node load, and when a refresh fails the last good
// snapshot keeps serving.
type Handler struct {
	client          Client
	storage         Storage
	logger          *zap.Logger
	snapshot        atomic.Pointer[snapshot]
	denoms          []string
	windows         []std.VestingSchedule
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

func NewHandler(client Client, store Storage, opts ...Option) *Handler {
	h := &Handler{
		client:          client,
		storage:         store,
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
	if err := h.loadWindows(); err != nil {
		return fmt.Errorf("unable to load the genesis vesting windows: %w", err)
	}

	ticker := time.NewTicker(h.refreshInterval)
	defer ticker.Stop()

	for {
		// The first refresh runs before the first tick, so the endpoint
		// has an answer right away instead of after one interval.
		h.refresh(ctx)

		select {
		case <-ctx.Done():
			h.logger.Info("Supply handler shut down")

			return nil
		case <-ticker.C:
		}
	}
}

// loadWindows reads the genesis balance rows the bootstrap stored and folds
// them into the vesting schedules the locked math needs.
func (h *Handler) loadWindows() error {
	balances, err := h.storage.GetGenesisBalances()
	if err != nil {
		return fmt.Errorf("unable to read the genesis balances: %w", err)
	}

	windows, err := FoldVestingWindows(balances)
	if err != nil {
		return err
	}

	h.windows = windows

	h.logger.Info("Loaded the genesis vesting windows", zap.Int("windows", len(windows)))

	return nil
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

// refresh rebuilds the snapshot. All reads happen at one chain height,
// in one batched round trip. The context comes from Start's lifecycle
// context with a timeout on top: a shutdown cancels an in-flight
// refresh instead of waiting out the timeout, and no request can ever
// wait on it or fail it. If the rebuild fails, the previous snapshot
// keeps serving. A panic is caught and logged, because a background
// loop must not take the indexer down.
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

	// One batch at one height, so every total comes from the same block.
	// Only the per-denom supply counters are read: the locked portion comes
	// from the genesis schedules alone, so a refresh scales with the denom
	// count and not with the number of vesting accounts. These round trips
	// belong to the handler, never to a request.
	paths := make([]string, 0, len(h.denoms))
	for _, denom := range h.denoms {
		paths = append(paths, "bank/supply/"+denom)
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

	// Locked is the still-unvested amount of every vesting window in the
	// genesis, computed from its summed schedule and the block time alone. Fees and
	// storage deposits bypass the lock and can debit an account below
	// its schedule, so this can overstate locked by what vesting
	// accounts have already spent. That is accepted in exchange for a
	// refresh that never reads a per-account balance. Each schedule is
	// evaluated once, whatever the number of tracked denoms.
	locked := make(map[string]int64, len(h.denoms))

	for _, window := range h.windows {
		unvested := window.LockedCoins(blockTime)

		for _, denom := range h.denoms {
			amount := unvested.AmountOf(denom)
			if amount <= 0 {
				continue
			}

			sum, ok := overflow.Add(locked[denom], amount)
			if !ok {
				sum = locked[denom] // skip the unrepresentable remainder rather than go negative
			}

			locked[denom] = sum
		}
	}

	wasClamped := h.clampedDenoms()

	supplies := make(map[string]*methods.Supply, len(h.denoms))
	clamped := make(map[string]bool, len(h.denoms))

	for _, denom := range h.denoms {
		total := totals[denom]

		lockedAmount := locked[denom]
		if lockedAmount > total {
			// The schedules name more than the counter holds, so vesting
			// accounts spent below their schedules. Nothing can be locked
			// beyond what exists: clamp, so that total = spendable + locked
			// keeps holding, and say so once rather than on every refresh
			// the disagreement lasts.
			clamped[denom] = true

			if !wasClamped[denom] {
				h.logger.Warn(
					"locked exceeds the supply counter, clamping",
					zap.String("denom", denom),
					zap.Int64("locked", lockedAmount),
					zap.Int64("total", total),
				)
			}

			lockedAmount = total
		} else if wasClamped[denom] {
			h.logger.Info(
				"locked is within the supply counter again",
				zap.String("denom", denom),
				zap.Int64("locked", lockedAmount),
				zap.Int64("total", total),
			)
		}

		supplies[denom] = &methods.Supply{
			Denom:     denom,
			Height:    height,
			Total:     total,
			Spendable: total - lockedAmount,
			Locked:    lockedAmount,
		}
	}

	return &snapshot{supplies: supplies, clamped: clamped}, nil
}

// clampedDenoms reports which denoms the latest snapshot capped at the supply
// counter; nil before the first snapshot.
func (h *Handler) clampedDenoms() map[string]bool {
	if previous := h.snapshot.Load(); previous != nil {
		return previous.clamped
	}

	return nil
}

func decodeSupply(res *core_types.ResultABCIQuery) (int64, error) {
	if err := responseError(res); err != nil {
		return 0, err
	}

	var total int64

	if err := amino.UnmarshalJSON(res.Response.Data, &total); err != nil {
		return 0, fmt.Errorf("unable to decode supply: %w", err)
	}

	if total < 0 {
		// A supply counts coins; a negative one is a malformed answer, and
		// publishing it would make locked negative too.
		return 0, fmt.Errorf("negative supply %d", total)
	}

	return total, nil
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
