package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAfterGenesis_WaitsForBootstrap makes sure a service that reads the
// genesis from the storage only starts once the bootstrap has put it there.
func TestAfterGenesis_WaitsForBootstrap(t *testing.T) {
	t.Parallel()

	var (
		ready   = make(chan struct{})
		started = make(chan struct{})
	)

	go func() {
		require.NoError(t, afterGenesis(ready, func(context.Context) error {
			close(started)

			return nil
		})(t.Context()))
	}()

	select {
	case <-started:
		require.FailNow(t, "the service started before the genesis was bootstrapped")
	case <-time.After(50 * time.Millisecond):
	}

	close(ready)

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the service never started")
	}
}

// TestAfterGenesis_ShutdownBeforeBootstrap makes sure a shutdown while the
// bootstrap is still retrying never starts the service, and stops cleanly.
func TestAfterGenesis_ShutdownBeforeBootstrap(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.NoError(t, afterGenesis(make(chan struct{}), func(context.Context) error {
		require.FailNow(t, "the service must not start")

		return nil
	})(ctx))
}

// TestStartCfg_QueryDeadlineFlag makes sure the query deadline is off unless
// the operator sets it, and takes a Go duration.
func TestStartCfg_QueryDeadlineFlag(t *testing.T) {
	t.Parallel()

	unset := &startCfg{}
	unsetFlags := flag.NewFlagSet("start", flag.ContinueOnError)
	unset.registerFlags(unsetFlags)

	require.NoError(t, unsetFlags.Parse(nil))
	assert.Zero(t, unset.queryDeadline)

	set := &startCfg{}
	setFlags := flag.NewFlagSet("start", flag.ContinueOnError)
	set.registerFlags(setFlags)

	require.NoError(t, setFlags.Parse([]string{"--query-deadline", "10s"}))
	assert.Equal(t, 10*time.Second, set.queryDeadline)
}

// TestStartCfg_NegativeQueryDeadline makes sure a negative deadline fails at
// startup rather than run with the deadline silently off.
func TestStartCfg_NegativeQueryDeadline(t *testing.T) {
	t.Parallel()

	cfg := &startCfg{
		queryDeadline: -time.Second,
		dbPath:        t.TempDir(),
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorContains(t, cfg.exec(ctx), "--query-deadline")
}

// TestStartCfg_MetricsListenAddressFlag makes sure the metrics endpoint is off
// unless the operator gives it an address.
func TestStartCfg_MetricsListenAddressFlag(t *testing.T) {
	t.Parallel()

	unset := &startCfg{}
	unsetFlags := flag.NewFlagSet("start", flag.ContinueOnError)
	unset.registerFlags(unsetFlags)

	require.NoError(t, unsetFlags.Parse(nil))
	assert.Empty(t, unset.metricsListenAddress)

	set := &startCfg{}
	setFlags := flag.NewFlagSet("start", flag.ContinueOnError)
	set.registerFlags(setFlags)

	require.NoError(t, setFlags.Parse([]string{"--metrics-listen-address", "127.0.0.1:9100"}))
	assert.Equal(t, "127.0.0.1:9100", set.metricsListenAddress)
}

// pickAddress returns a loopback address no listener holds at the time. The
// start command binds its addresses itself, so another process can take one
// before it does: startIndexer then starts it again on fresh ones.
func pickAddress(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	address := ln.Addr().String()
	require.NoError(t, ln.Close())

	return address
}

// testCfg is the start command's configuration with args parsed over its
// defaults, on a fresh database, against a chain that never answers unless
// args name one
func testCfg(t *testing.T, args ...string) *startCfg {
	t.Helper()

	cfg := &startCfg{}
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	cfg.registerFlags(fs)

	require.NoError(t, fs.Parse(append([]string{
		"--db-path", t.TempDir(),
		// No chain listens there: the genesis bootstrap retries until the
		// test ends, while the API serves the empty database
		"--remote", "http://127.0.0.1:1",
		// The bootstrap's retries would fill the test output
		"--log-level", "fatal",
	}, args...)))

	return cfg
}

// indexer is a start command running for a test, on the addresses it serves
// its API and, when asked, its metrics on
type indexer struct {
	apiAddress     string
	metricsAddress string
}

// startAttempts bounds how often startIndexer picks fresh addresses for a
// start command that found one of its addresses taken
const startAttempts = 5

// startIndexer runs the start command with args until the test ends, serving
// its metrics too with serveMetrics, and returns once every listener answers
func startIndexer(t *testing.T, serveMetrics bool, args ...string) indexer {
	t.Helper()

	for range startAttempts {
		if started, ok := tryStartIndexer(t, serveMetrics, args); ok {
			return started
		}
	}

	require.FailNow(t, "every address picked for the indexer was taken before it bound it")

	return indexer{}
}

// tryStartIndexer starts the command once on fresh addresses, and reports
// false when it stopped on one of them taken before it bound it
func tryStartIndexer(t *testing.T, serveMetrics bool, args []string) (indexer, bool) {
	t.Helper()

	started := indexer{apiAddress: pickAddress(t)}
	flags := []string{"--listen-address", started.apiAddress}

	if serveMetrics {
		started.metricsAddress = pickAddress(t)
		flags = append(flags, "--metrics-listen-address", started.metricsAddress)
	}

	cfg := testCfg(t, append(flags, args...)...)

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)

	go func() {
		stopped <- cfg.exec(ctx)
	}()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case err := <-stopped:
			cancel()

			if errors.Is(err, syscall.EADDRINUSE) {
				return indexer{}, false
			}

			require.FailNow(t, "the indexer stopped before it served", "%v", err)
		case <-timeout.C:
			cancel()
			<-stopped

			require.FailNow(t, "the indexer never served")
		case <-ticker.C:
			if !started.serves(t) {
				continue
			}

			t.Cleanup(func() {
				cancel()

				select {
				case err := <-stopped:
					assert.NoError(t, withoutStderrSync(err))
				case <-time.After(10 * time.Second):
					assert.Fail(t, "the indexer did not stop")
				}
			})

			return started, true
		}
	}
}

// serves reports whether every listener of the indexer answers
func (i indexer) serves(t *testing.T) bool {
	t.Helper()

	if get(t, "http://"+i.apiAddress+"/health") == nil {
		return false
	}

	return i.metricsAddress == "" || get(t, "http://"+i.metricsAddress+"/metrics") != nil
}

// An address another listener holds stops the start command with an error
// that says so: startIndexer reads it to start again on fresh addresses
func TestStartCfg_TakenAddressStopsTheCommand(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, ln.Close())
	})

	cfg := testCfg(t, "--listen-address", ln.Addr().String())

	stopped := make(chan error, 1)

	go func() {
		stopped <- cfg.exec(context.Background())
	}()

	select {
	case err := <-stopped:
		require.ErrorIs(t, err, syscall.EADDRINUSE)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "the indexer did not stop on its taken address")
	}
}

// withoutStderrSync is what the start command returned at shutdown, less the
// error of its logger's sync. The command returns the error logger.Sync
// returns, and zap fails to sync a stderr that is not a terminal, as under go
// test or a container's log pipe: a clean stop of the indexer ends on that
// error, and main exits 1. The test drops that one error and fails on any
// other.
func withoutStderrSync(err error) error {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return err
	}

	var rest []error

	for _, e := range joined.Unwrap() {
		if !strings.HasPrefix(e.Error(), "sync /dev/stderr") {
			rest = append(rest, e)
		}
	}

	return errors.Join(rest...)
}

// get fetches url and returns the response with its body read, or nil if
// nothing answers there
func get(t *testing.T, url string) *httpResponse {
	t.Helper()

	return fetchURL(t, http.MethodGet, url, "", "")
}

// post sends body to url as JSON, and requires an OK answer
func post(t *testing.T, url, body string) {
	t.Helper()

	response := fetchURL(t, http.MethodPost, url, "application/json", body)
	require.NotNil(t, response, "nothing answers at %s", url)
	require.Equal(t, http.StatusOK, response.status, response.body)
}

// httpResponse is an answer with its body read
type httpResponse struct {
	body   string
	status int
}

// fetchURL sends one request and returns its answer, or nil if nothing
// answers at url
func fetchURL(t *testing.T, method, url, contentType, body string) *httpResponse {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	require.NoError(t, err)

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}

	defer func() {
		assert.NoError(t, resp.Body.Close())
	}()

	read, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return &httpResponse{status: resp.StatusCode, body: string(read)}
}

// The metrics are served on their own listener and record the requests both
// APIs serve; the public API never serves them
func TestStartCfg_ServesMetricsApartFromTheAPI(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, true)

	post(t, "http://"+started.apiAddress+"/", `{"jsonrpc": "2.0", "id": 1, "method": "getBlock", "params": ["1"]}`)
	post(t, "http://"+started.apiAddress+"/graphql/query", `{"query": "{ getBlocks(where: {}) { height } }"}`)

	// The calls are recorded before they are answered, and startIndexer has
	// seen the metrics listener answer: one scrape holds them
	scrape := get(t, "http://"+started.metricsAddress+"/metrics")
	require.NotNil(t, scrape, "nothing serves the metrics")
	require.Equal(t, http.StatusOK, scrape.status)

	assert.Contains(t, scrape.body, `tx_indexer_requests_total{api="jsonrpc",operation="getBlock",status="ok"} 1`)
	assert.Contains(t, scrape.body, `tx_indexer_requests_total{api="graphql",operation="getBlocks",status="ok"} 1`)

	onAPI := get(t, "http://"+started.apiAddress+"/metrics")
	require.NotNil(t, onAPI)
	assert.NotContains(t, onAPI.body, "tx_indexer_")
}

// Without a metrics address, the default, the public API still never serves
// the metrics
func TestStartCfg_MetricsAreOffByDefault(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false)

	post(t, "http://"+started.apiAddress+"/", `{"jsonrpc": "2.0", "id": 1, "method": "getBlock", "params": ["1"]}`)

	onAPI := get(t, "http://"+started.apiAddress+"/metrics")
	require.NotNil(t, onAPI)
	assert.NotEqual(t, http.StatusOK, onAPI.status)
	assert.NotContains(t, onAPI.body, "tx_indexer_")
}
