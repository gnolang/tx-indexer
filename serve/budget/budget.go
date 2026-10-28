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

// RetryAfterSeconds is RetryAfter in whole seconds, as Retry-After carries it
func (e *ExhaustedError) RetryAfterSeconds() int64 {
	return int64(e.RetryAfter / time.Second)
}

// Details are the error's machine-readable fields, carried beside the message
// in the GraphQL error's extensions and the JSON-RPC error's data, as the
// query deadline's are. Clients read them rather than the message text.
func (e *ExhaustedError) Details() map[string]any {
	return map[string]any{
		"code":              "CLIENT_TIME_BUDGET_EXHAUSTED",
		"retryAfterSeconds": e.RetryAfterSeconds(),
	}
}

// Client is the client that opened a WebSocket connection, as a budget
// charges it: each query or call the connection carries is charged to it
type Client struct {
	// Budget is the budget the client is charged in
	Budget *Budget

	// Key is the key the client is charged under
	Key string

	// Address is the client's address, the one the request log names
	Address string
}

// clientKey is the context key a connection's Client is carried under
type clientKey struct{}

// NewContext carries client in ctx, the context of the requests a
// connection carries
func NewContext(ctx context.Context, client Client) context.Context {
	return context.WithValue(ctx, clientKey{}, client)
}

// FromContext is the Client ctx carries, and whether it carries one
func FromContext(ctx context.Context) (Client, bool) {
	client, ok := ctx.Value(clientKey{}).(Client)

	return client, ok && client.Budget != nil
}

// Reserve admits a request of the client, as Budget.Reserve does
func (c Client) Reserve() (func(), error) {
	return c.Budget.Reserve(c.Key)
}

// sweepInterval is how often Run forgets the clients whose balance is full
const sweepInterval = time.Minute

// Budget holds the balance of indexer time of each client, by the key the
// client is charged under. It is safe for concurrent use.
//
// A request is admitted while its client's balance, less one query deadline
// for each of the client's requests in flight, is above zero, and is charged
// the wall time it took once it ends. The deadline a request holds while it
// runs keeps its client's requests in flight to what the balance covers. What
// a request overdraws is the part of its wall time the deadline does not
// bound: the deadline bounds a query's execution, not its upload, its
// parsing, its validation or the writing of its answer. A client whose
// balance is full, with nothing in flight, is forgotten, so the budget holds
// the clients active within one refill of it.
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

	// balance is the indexer time left to the client, in nanoseconds, below
	// zero once its requests overdrew it. It counts fractions of a nanosecond,
	// so a refill too slow to add a whole one between two requests is not
	// lost.
	balance float64

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

// Config is a budget's settings, each above zero: the start command refuses
// any other
type Config struct {
	// Limit is the most indexer time a client holds
	Limit time.Duration

	// Refill is the indexer time a client regains per minute
	Refill time.Duration

	// Deadline is the query deadline, held of a client's balance for each
	// of its requests in flight
	Deadline time.Duration
}

// New creates a budget of indexer time per client, as config sets it
func New(config Config, opts ...Option) *Budget {
	b := &Budget{
		now:      time.Now,
		clients:  make(map[string]*account),
		limit:    config.Limit,
		refill:   config.Refill,
		deadline: config.Deadline,
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
		a = &account{updated: now, balance: float64(b.limit)}
	}

	b.refillTo(a, now)

	available := a.balance - float64(a.inFlight)*float64(b.deadline)
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

	a.balance -= float64(now.Sub(admitted))
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

	a.balance = min(a.balance+float64(elapsed)*b.refillRate(), float64(b.limit))
	a.updated = now
}

// forgetIfFull forgets client, holding a, if its balance is full and nothing
// of it is in flight: it is then a client the budget has never seen
func (b *Budget) forgetIfFull(client string, a *account) {
	if a.inFlight == 0 && a.balance >= float64(b.limit) {
		delete(b.clients, client)
	}
}

// maxRetryAfterSeconds is the longest Retry-After a duration holds, in whole
// seconds
const maxRetryAfterSeconds = math.MaxInt64 / int64(time.Second)

// retryAfter is the whole seconds until available, the balance less what the
// requests in flight hold, in nanoseconds, refills above zero: at least one,
// since a balance at zero is above it only a moment later, and at most what a
// duration holds, which a refill too slow for what the client holds exceeds
func (b *Budget) retryAfter(available float64) time.Duration {
	wait := -available / b.refillRate()
	seconds := min(math.Floor(wait/float64(time.Second))+1, float64(maxRetryAfterSeconds))

	return time.Duration(seconds) * time.Second
}

// refillRate is the indexer time a client regains per unit of time
func (b *Budget) refillRate() float64 {
	return float64(b.refill) / float64(time.Minute)
}
