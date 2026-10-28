// Package budget gives each client a budget of indexer time: the wall time
// its requests may hold the indexer, spent as they are served and regained at
// a steady rate, so no client's queries hold the indexer from the others for
// long.
package budget

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// ExhaustedError is the error a request is refused with once its client has
// spent its budget
type ExhaustedError struct {
	// RetryAfter is how long until the client is served again: whole
	// seconds, at least one
	RetryAfter time.Duration
}

func (e *ExhaustedError) Error() string {
	return fmt.Sprintf("client time budget exhausted; retry in %s", e.RetryAfter)
}

// Details are the error's machine-readable fields, carried beside the message
// in the GraphQL error's extensions and the JSON-RPC error's data, as the
// query deadline's are. Clients read them rather than the message text.
func (e *ExhaustedError) Details() map[string]any {
	return map[string]any{
		"code":              "CLIENT_TIME_BUDGET_EXHAUSTED",
		"retryAfterSeconds": int(e.RetryAfter / time.Second),
	}
}

// sweepInterval is how often Run forgets the clients whose balance is full
const sweepInterval = time.Minute

// Budget holds the balance of indexer time of each client, by the key the
// client is charged under. It is safe for concurrent use.
//
// A request is admitted while its client's balance, less one query deadline
// for each of the client's requests in flight, is above zero, and is charged
// the wall time it took once it ends. The deadline a request holds while it
// runs bounds a client's overdraw to one deadline, however many requests it
// sends at once. A client whose balance is full, with nothing in flight, is
// forgotten, so the budget holds the clients active within one refill of it.
type Budget struct {
	// now is the clock the balances are kept by
	now func() time.Time

	// clients are the clients whose balance is not full, or who have a
	// request in flight
	clients map[string]*account

	// limit is the most indexer time a client holds
	limit time.Duration

	// refill is the indexer time a client regains per minute
	refill time.Duration

	// deadline is the query deadline, held of the balance for each request
	// in flight
	deadline time.Duration

	mu sync.Mutex
}

// account is one client's balance
type account struct {
	// updated is when balance was last brought up to date
	updated time.Time

	// balance is the indexer time left to the client, below zero once its
	// requests overdrew it
	balance time.Duration

	// inFlight counts the client's requests admitted and not yet ended
	inFlight int
}

// Option configures a Budget
type Option func(b *Budget)

// WithClock keeps the balances by now in place of the wall clock, for tests
func WithClock(now func() time.Time) Option {
	return func(b *Budget) {
		b.now = now
	}
}

// New creates a budget of limit indexer time per client, of which refill is
// regained per minute, for requests bounded by the query deadline deadline.
// Each is above zero: the start command refuses any other.
func New(limit, refill, deadline time.Duration, opts ...Option) *Budget {
	b := &Budget{
		now:      time.Now,
		clients:  make(map[string]*account),
		limit:    limit,
		refill:   refill,
		deadline: deadline,
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// Reserve admits a request of client while the client's balance, less one
// query deadline for each of its requests in flight, is above zero. It returns
// the function that ends the request, to call once when it has been served:
// the request is then charged the time since its admission. A request it
// does not admit is refused with an *ExhaustedError.
func (b *Budget) Reserve(client string) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()

	a, ok := b.clients[client]
	if !ok {
		a = &account{updated: now, balance: b.limit}
	}

	b.refillTo(a, now)

	available := a.balance - time.Duration(a.inFlight)*b.deadline
	if available <= 0 {
		return nil, &ExhaustedError{RetryAfter: b.retryAfter(available)}
	}

	a.inFlight++
	b.clients[client] = a

	return func() {
		b.end(client, a, now)
	}, nil
}

// end charges a request of client, admitted at admitted, the time since, and
// forgets the client if its balance is full and nothing of it is in flight
func (b *Budget) end(client string, a *account, admitted time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()

	b.refillTo(a, now)

	a.balance -= now.Sub(admitted)
	a.inFlight--

	b.forgetIfFull(client, a)
}

// Sweep forgets every client whose balance is full, with nothing in flight
func (b *Budget) Sweep() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()

	for client, a := range b.clients {
		b.refillTo(a, now)
		b.forgetIfFull(client, a)
	}
}

// Run sweeps the budget every minute, so the clients that left are forgotten
// once their balance has refilled. It returns once ctx is done.
func (b *Budget) Run(ctx context.Context) error {
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			b.Sweep()
		}
	}
}

// refillTo brings a's balance up to now: the refill per minute for the time
// since it was last brought up to date, never past the budget
func (b *Budget) refillTo(a *account, now time.Time) {
	elapsed := now.Sub(a.updated)
	if elapsed <= 0 {
		return
	}

	balance := float64(a.balance) + float64(elapsed)*b.refillRate()

	a.balance = time.Duration(min(balance, float64(b.limit)))
	a.updated = now
}

// forgetIfFull forgets client, holding a, if its balance is full and nothing
// of it is in flight: it is then a client the budget has never seen
func (b *Budget) forgetIfFull(client string, a *account) {
	if a.inFlight == 0 && a.balance >= b.limit {
		delete(b.clients, client)
	}
}

// retryAfter is the whole seconds until available, the balance less what the
// requests in flight hold, refills above zero: at least one, since a balance
// at zero is above it only a moment later
func (b *Budget) retryAfter(available time.Duration) time.Duration {
	wait := float64(-available) / b.refillRate()

	return time.Duration(math.Floor(wait/float64(time.Second))+1) * time.Second
}

// refillRate is the indexer time a client regains per unit of time
func (b *Budget) refillRate() float64 {
	return float64(b.refill) / float64(time.Minute)
}
