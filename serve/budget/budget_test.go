package budget

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The budget the tests run with: 60 s of indexer time per client, regained at
// 30 s a minute, with a 30 s query deadline
const (
	testLimit    = 60 * time.Second
	testRefill   = 30 * time.Second
	testDeadline = 30 * time.Second
)

// testConfig is the budget the tests run with
var testConfig = Config{Limit: testLimit, Refill: testRefill, Deadline: testDeadline}

// testClient is the key the tests charge
const testClient = "203.0.113.9"

// fakeClock is a clock the test moves
type fakeClock struct {
	now time.Time
}

// Now is the clock's time
func (c *fakeClock) Now() time.Time {
	return c.now
}

// advance moves the clock by d
func (c *fakeClock) advance(d time.Duration) {
	c.now = c.now.Add(d)
}

// newTestBudget is the tests' budget on a fake clock
func newTestBudget() (*Budget, *fakeClock) {
	clock := &fakeClock{now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)}

	return New(testConfig, WithClock(clock.Now)), clock
}

// balanceOf is testClient's balance as of the budget's clock, which must hold
// the client
func balanceOf(t *testing.T, b *Budget) time.Duration {
	t.Helper()

	b.mu.Lock()
	defer b.mu.Unlock()

	a, ok := b.clients[testClient]
	require.True(t, ok, "the budget forgot the client")

	b.refillTo(a, b.now())

	return time.Duration(a.balance)
}

// clientCount is how many clients the budget holds
func clientCount(b *Budget) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.clients)
}

// serve admits a request of testClient, holds it for d on the clock, and
// ends it
func serve(t *testing.T, b *Budget, clock *fakeClock, d time.Duration) {
	t.Helper()

	end, err := b.Reserve(testClient)
	require.NoError(t, err)

	clock.advance(d)
	end()
}

// retryAfter is the Retry-After of err, which must be an *ExhaustedError
func retryAfter(t *testing.T, err error) time.Duration {
	t.Helper()

	var exhausted *ExhaustedError
	require.ErrorAs(t, err, &exhausted)

	return exhausted.RetryAfter
}

// A request is admitted while its client's balance is above zero, and refused
// once it is at zero or below
func TestBudget_AdmitsWhileTheBalanceIsAboveZero(t *testing.T) {
	t.Parallel()

	b, clock := newTestBudget()

	// The request runs while the balance is full: the time it regains then is
	// lost, and the whole minute is charged
	serve(t, b, clock, time.Minute)

	balance := balanceOf(t, b)
	require.Equal(t, time.Duration(0), balance)

	_, err := b.Reserve(testClient)
	require.Error(t, err)

	clock.advance(time.Millisecond)

	end, err := b.Reserve(testClient)
	require.NoError(t, err)

	end()
}

// The time from a request's admission to its end is charged to its client's
// balance, which then regains the refill per minute
func TestBudget_ChargesTheTimeServed(t *testing.T) {
	t.Parallel()

	b, clock := newTestBudget()

	serve(t, b, clock, 25*time.Second)

	balance := balanceOf(t, b)
	assert.Equal(t, 35*time.Second, balance)

	clock.advance(10 * time.Second)

	balance = balanceOf(t, b)
	assert.Equal(t, 40*time.Second, balance)
}

// The balance never refills past the budget, however long the client stays
// away
func TestBudget_CapsTheRefillAtTheBudget(t *testing.T) {
	t.Parallel()

	b, clock := newTestBudget()

	// Held in flight, the client stays in the budget while its balance refills
	end, err := b.Reserve(testClient)
	require.NoError(t, err)

	clock.advance(time.Hour)

	balance := balanceOf(t, b)
	assert.Equal(t, testLimit, balance)

	end()
}

// A refusal names the whole seconds until the balance, less what the
// requests in flight hold, is back above zero: rounded up, and at least one
func TestBudget_RetryAfterRoundsUpToWholeSeconds(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name string

		// spent is the time the one request served before the refusal took
		spent time.Duration

		// want is the Retry-After; at want less one second, the client is
		// still refused, and at want it is served
		want time.Duration
	}{
		// 2.2 s overdrawn regain in 4.4 s at 0.5 s a second
		{"a fraction of a second", testLimit + 2200*time.Millisecond, 5 * time.Second},
		// 2.5 s overdrawn regain in 5 s, when the balance is zero, not above
		{"a whole number of seconds", testLimit + 2500*time.Millisecond, 6 * time.Second},
		// a balance at zero is above it a moment later
		{"a balance at zero", testLimit, time.Second},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			b, clock := newTestBudget()

			serve(t, b, clock, testCase.spent)

			_, err := b.Reserve(testClient)
			assert.Equal(t, testCase.want, retryAfter(t, err))

			clock.advance(testCase.want - time.Second)

			_, err = b.Reserve(testClient)
			require.Error(t, err, "served a second before its Retry-After")

			clock.advance(time.Second)

			end, err := b.Reserve(testClient)
			require.NoError(t, err, "refused at its Retry-After")

			end()
		})
	}
}

// Each request in flight holds one query deadline of its client's balance
// until it ends, so the client overdraws one deadline at most: with a 60 s
// budget and a 30 s deadline, a third request is refused while two run
func TestBudget_ReservesTheDeadlineOfEachRequestInFlight(t *testing.T) {
	t.Parallel()

	b, _ := newTestBudget()

	endFirst, err := b.Reserve(testClient)
	require.NoError(t, err)

	endSecond, err := b.Reserve(testClient)
	require.NoError(t, err)

	_, err = b.Reserve(testClient)
	assert.Equal(t, time.Second, retryAfter(t, err))

	endFirst()

	endThird, err := b.Reserve(testClient)
	require.NoError(t, err)

	endSecond()
	endThird()
}

// Clients are charged apart
func TestBudget_ChargesEachClientApart(t *testing.T) {
	t.Parallel()

	b, clock := newTestBudget()

	serve(t, b, clock, 2*time.Minute)

	_, err := b.Reserve(testClient)
	require.Error(t, err)

	end, err := b.Reserve("203.0.113.10")
	require.NoError(t, err)

	end()
}

// A client whose balance is full, with nothing in flight, is forgotten: at
// once when its last request ends so, and otherwise by a sweep once its
// balance has refilled
func TestBudget_ForgetsAFullClient(t *testing.T) {
	t.Parallel()

	b, clock := newTestBudget()

	serve(t, b, clock, 0)
	assert.Zero(t, clientCount(b))

	serve(t, b, clock, 30*time.Second)

	b.Sweep()
	assert.Equal(t, 1, clientCount(b))

	// 30 s overdrawn from the full budget regain in a minute
	clock.advance(time.Minute)
	b.Sweep()
	assert.Zero(t, clientCount(b))

	end, err := b.Reserve(testClient)
	require.NoError(t, err)

	b.Sweep()
	assert.Equal(t, 1, clientCount(b), "a client in flight was forgotten")

	end()
}

// Run sweeps the full clients once a minute until its context ends
func TestBudget_RunSweepsEachMinute(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := New(testConfig)

		end, err := b.Reserve(testClient)
		require.NoError(t, err)

		time.Sleep(30 * time.Second)
		end()

		ctx, cancel := context.WithCancel(t.Context())
		stopped := make(chan error, 1)

		go func() {
			stopped <- b.Run(ctx)
		}()

		// 30 s overdrawn regain in a minute, when Run sweeps first
		time.Sleep(time.Minute + time.Second)
		assert.Zero(t, clientCount(b))

		cancel()
		require.NoError(t, <-stopped)
	})
}

// The budget is safe under requests of many clients at once
func TestBudget_IsSafeUnderConcurrency(t *testing.T) {
	t.Parallel()

	b := New(testConfig)

	var wg sync.WaitGroup

	for i := range 50 {
		wg.Go(func() {
			for range 20 {
				end, err := b.Reserve(fmt.Sprintf("203.0.113.%d", i%5))
				if err == nil {
					end()
				}
			}
		})

		wg.Go(b.Sweep)
	}

	wg.Wait()

	b.mu.Lock()
	defer b.mu.Unlock()

	for client, a := range b.clients {
		assert.Zero(t, a.inFlight, client)
	}
}

// A refusal names itself and its Retry-After to people and to programs
func TestExhaustedError(t *testing.T) {
	t.Parallel()

	var err error = &ExhaustedError{RetryAfter: 5 * time.Second}

	assert.Equal(t, "client time budget exhausted; retry in 5s", err.Error())

	var exhausted *ExhaustedError
	require.True(t, errors.As(err, &exhausted))

	assert.Equal(t, map[string]any{
		"code":              "CLIENT_TIME_BUDGET_EXHAUSTED",
		"retryAfterSeconds": 5,
	}, exhausted.Details())
}

// A refill too slow to add a whole nanosecond between two reads of a balance
// is not lost, and a balance overdrawn gains nothing by being read: a client
// asking every second for a minute is refused no sooner than one that asks
// once
func TestBudget_KeepsFractionsOfANanosecond(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)}

	// 60 ns of indexer time, regained at 1 ns a minute
	b := New(Config{Limit: 60, Refill: 1, Deadline: 60}, WithClock(clock.Now))

	end, err := b.Reserve(testClient)
	require.NoError(t, err)

	clock.advance(100)
	end()

	// 40 ns overdrawn regain in 40 minutes
	_, err = b.Reserve(testClient)
	assert.Equal(t, 40*time.Minute+time.Second, retryAfter(t, err))

	for range 60 {
		clock.advance(time.Second)

		_, err = b.Reserve(testClient)
		require.Error(t, err)
	}

	assert.Equal(t, 39*time.Minute+time.Second, retryAfter(t, err))
}

// A wait too long for a duration, from a refill too slow for what the client
// holds, is answered with the longest Retry-After a duration holds, never one
// that wrapped around below zero
func TestBudget_BoundsRetryAfter(t *testing.T) {
	t.Parallel()

	// 1 ns regained a minute, against 30 s held by a request in flight
	b := New(Config{Limit: 1, Refill: 1, Deadline: 30 * time.Second})

	end, err := b.Reserve(testClient)
	require.NoError(t, err)

	defer end()

	_, err = b.Reserve(testClient)
	assert.Equal(t, time.Duration(math.MaxInt64).Truncate(time.Second), retryAfter(t, err))
}
