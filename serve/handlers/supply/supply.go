package supply

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	"github.com/gnolang/gno/tm2/pkg/amino"
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/crypto"
	"github.com/gnolang/gno/tm2/pkg/overflow"
	"github.com/gnolang/gno/tm2/pkg/std"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/methods"
	"github.com/gnolang/tx-indexer/serve/spec"
)

// defaultCacheTTL bounds how often the chain is re-queried for one denom.
// Aggregators poll on the order of minutes; blocks are on the order of
// seconds, so a short TTL keeps the answer fresh without letting request
// traffic reach the chain.
const defaultCacheTTL = 10 * time.Second

// Option is a functional option for the supply Handler.
type Option func(*Handler)

// WithLogger sets the handler logger.
func WithLogger(logger *zap.Logger) Option {
	return func(h *Handler) {
		h.logger = logger
	}
}

// WithCacheTTL overrides how long one computed supply stays cached.
func WithCacheTTL(ttl time.Duration) Option {
	return func(h *Handler) {
		h.cacheTTL = ttl
	}
}

// Client is the chain access the supply handler needs. Small on purpose:
// the handler must stay testable against a mock, and the fetcher's Client
// carries fetching concerns this handler does not have.
type Client interface {
	// GetGenesis returns the chain genesis, where vesting schedules live
	GetGenesis(context.Context) (*core_types.ResultGenesis, error)

	// GetStatus returns the chain status, for the height and block time
	// the spendability split is evaluated at
	GetStatus(context.Context) (*core_types.ResultStatus, error)

	// ABCIQuery runs an ABCI query against the chain
	ABCIQuery(ctx context.Context, path string, data []byte) (*core_types.ResultABCIQuery, error)
}

// vestingEntry is one genesis vesting account, with the schedule pre-built
// into an account so the locked math is the chain's own.
type vestingEntry struct {
	account std.VestingAccount
	address crypto.Address
}

type cachedSupply struct {
	expires time.Time
	supply  *methods.Supply
}

type Handler struct {
	client     Client
	sf         singleflight.Group
	vestingErr error
	logger     *zap.Logger
	cache      map[string]cachedSupply
	vesting    []vestingEntry
	cacheTTL   time.Duration
	sync.Once
	mu sync.Mutex
}

func NewHandler(client Client, opts ...Option) *Handler {
	h := &Handler{
		client:   client,
		logger:   zap.NewNop(),
		cache:    make(map[string]cachedSupply),
		cacheTTL: defaultCacheTTL,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// GetSupplyHandler returns the supply of a single denomination, split into
// total, spendable and locked, at the latest chain height.
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

	// Validated locally so a typo fails before any chain round trip.
	if err := std.ValidateDenom(denom); err != nil {
		return nil, spec.GenerateInvalidParamError(1)
	}

	supply, err := h.GetSupply(context.Background(), denom)
	if err != nil {
		return nil, spec.GenerateResponseError(err)
	}

	return supply, nil
}

// GetSupply returns the supply of denom split into total, spendable and
// locked at the latest chain height. Cached briefly, and shared across
// concurrent callers, so heavy polling does not reach the chain.
func (h *Handler) GetSupply(ctx context.Context, denom string) (*methods.Supply, error) {
	h.mu.Lock()
	cached, ok := h.cache[denom]
	h.mu.Unlock()

	if ok && time.Now().Before(cached.expires) {
		return cached.supply, nil
	}

	value, err, _ := h.sf.Do(denom, func() (any, error) {
		// Re-check under the flight: the winner of a concurrent call may
		// have refreshed the entry while this one waited.
		h.mu.Lock()
		cached, ok := h.cache[denom]
		h.mu.Unlock()

		if ok && time.Now().Before(cached.expires) {
			return cached.supply, nil
		}

		supply, err := h.computeSupply(ctx, denom)
		if err != nil {
			return nil, err
		}

		h.mu.Lock()
		h.cache[denom] = cachedSupply{
			supply:  supply,
			expires: time.Now().Add(h.cacheTTL),
		}
		h.mu.Unlock()

		return supply, nil
	})
	if err != nil {
		return nil, err
	}

	return value.(*methods.Supply), nil
}

// computeSupply walks the chain for the total and, per vesting account, the
// live balance. Locked is the still-unvested amount clamped to the balance
// actually held — fees and storage refunds bypass the lock and can spend into
// the locked portion, and a lock on coins nobody holds locks nothing.
func (h *Handler) computeSupply(ctx context.Context, denom string) (*methods.Supply, error) {
	if err := h.loadVesting(ctx); err != nil {
		return nil, err
	}

	status, err := h.client.GetStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get chain status, %w", err)
	}

	height := status.SyncInfo.LatestBlockHeight
	blockTime := status.SyncInfo.LatestBlockTime

	total, err := h.queryTotal(ctx, denom)
	if err != nil {
		return nil, err
	}

	var locked int64

	for _, entry := range h.vesting {
		unvested := entry.account.LockedCoins(blockTime).AmountOf(denom)
		if unvested <= 0 {
			continue
		}

		balance, err := h.queryBalance(ctx, entry.address, denom)
		if err != nil {
			return nil, err
		}

		amount := min(balance, unvested)

		sum, ok := overflow.Add(locked, amount)
		if !ok {
			sum = locked // skip the unrepresentable remainder rather than go negative
		}

		locked = sum
	}

	spendable := total - locked
	if spendable < 0 {
		// The counter disagrees with the balances; report the floor rather
		// than a negative circulating supply.
		spendable = 0
	}

	return &methods.Supply{
		Denom:     denom,
		Height:    height,
		Total:     total,
		Spendable: spendable,
		Locked:    locked,
	}, nil
}

// loadVesting reads the vesting schedules from genesis, once. Vesting
// accounts are created only at genesis and schedules are immutable, so the
// data never goes stale; the chain collapses a fully vested account on its
// next spend, which the schedule math alone already reports as zero locked.
func (h *Handler) loadVesting(ctx context.Context) error {
	h.Do(func() {
		h.vesting, h.vestingErr = h.parseVesting(ctx)
	})

	return h.vestingErr
}

func (h *Handler) parseVesting(ctx context.Context) ([]vestingEntry, error) {
	genesis, err := h.client.GetGenesis(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch genesis, %w", err)
	}

	if genesis.Genesis == nil {
		return nil, fmt.Errorf("nil genesis doc")
	}

	state, ok := genesis.Genesis.AppState.(gnoland.GnoGenesisState)
	if !ok {
		return nil, fmt.Errorf("unexpected genesis app state type %T", genesis.Genesis.AppState)
	}

	var entries []vestingEntry

	for _, balance := range state.Balances {
		if !balance.IsVesting() {
			continue
		}

		account, err := newVestingAccount(balance.Address, balance.Vesting)
		if err != nil {
			// Fail rather than skip: a schedule the chain's own types
			// cannot rebuild would silently undercount locked.
			return nil, fmt.Errorf("invalid vesting balance for %s: %w", balance.Address, err)
		}

		entries = append(entries, vestingEntry{
			account: account,
			address: balance.Address,
		})
	}

	h.logger.Info("loaded vesting schedules from genesis", zap.Int("count", len(entries)))

	return entries, nil
}

// newVestingAccount rebuilds the chain's account for a genesis schedule, so
// the locked math is the chain's own implementation rather than a re-derived
// copy of it.
func newVestingAccount(addr crypto.Address, schedule *std.VestingSchedule) (std.VestingAccount, error) {
	base := std.NewBaseAccount(addr, schedule.OriginalVesting, nil, 0, 0)

	switch schedule.Type {
	case std.VestingDelayed:
		return std.NewDelayedVestingAccount(base, *schedule)
	default:
		return std.NewContinuousVestingAccount(base, *schedule)
	}
}

// queryTotal reads the chain's per-denom supply counter (bank/supply/<denom>).
func (h *Handler) queryTotal(ctx context.Context, denom string) (int64, error) {
	res, err := h.client.ABCIQuery(ctx, "bank/supply/"+denom, nil)
	if err != nil {
		return 0, fmt.Errorf("unable to query supply of %q, %w", denom, err)
	}

	if res.Response.Error != nil {
		// A chain without the bank/supply route reports it here; name the
		// route so the operator knows what is missing.
		return 0, fmt.Errorf("bank/supply/%s: %w", denom, res.Response.Error)
	}

	var total int64

	if err := amino.UnmarshalJSON(res.Response.Data, &total); err != nil {
		return 0, fmt.Errorf("unable to decode supply of %q, %w", denom, err)
	}

	return total, nil
}

// queryBalance reads one address's balance of denom (bank/balances/<addr>).
func (h *Handler) queryBalance(ctx context.Context, addr crypto.Address, denom string) (int64, error) {
	res, err := h.client.ABCIQuery(ctx, "bank/balances/"+crypto.AddressToBech32(addr), nil)
	if err != nil {
		return 0, fmt.Errorf("unable to query balance of %s, %w", addr, err)
	}

	if res.Response.Error != nil {
		return 0, fmt.Errorf("bank/balances/%s: %w", addr, res.Response.Error)
	}

	var coins std.Coins

	if err := amino.UnmarshalJSON(res.Response.Data, &coins); err != nil {
		return 0, fmt.Errorf("unable to decode balance of %s, %w", addr, err)
	}

	return coins.AmountOf(denom), nil
}
