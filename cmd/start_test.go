package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	ctypes "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	rpcserver "github.com/gnolang/gno/tm2/pkg/bft/rpc/lib/server"
	rpctypes "github.com/gnolang/gno/tm2/pkg/bft/rpc/lib/types"
	"github.com/gnolang/gno/tm2/pkg/bft/state"
	bftTypes "github.com/gnolang/gno/tm2/pkg/bft/types"
	p2pTypes "github.com/gnolang/gno/tm2/pkg/p2p/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve/observe"
	"github.com/gnolang/tx-indexer/storage"
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

	// The test's context ends on every way out of the test, a failure before
	// the Cleanup below is registered included, so the indexer never outlives
	// it; it ends before the Cleanup functions run, so that one still waits
	// for the stop
	ctx, cancel := context.WithCancel(t.Context())
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
					assert.NoError(t, err)
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

// sample returns the line of scrape that carries series, a metric name and
// its labels as the exposition format writes them, or "" if none does
func sample(scrape, series string) string {
	for line := range strings.Lines(scrape) {
		if strings.HasPrefix(line, series+" ") {
			return strings.TrimSuffix(line, "\n")
		}
	}

	return ""
}

// get fetches url and returns the response with its body read, or nil if
// nothing answers there
func get(t require.TestingT, url string) *httpResponse {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}

	return fetchURL(t, http.MethodGet, url, nil, "")
}

// post sends body to url as JSON, and requires an OK answer
func post(t *testing.T, url, body string) {
	t.Helper()

	response := fetchURL(t, http.MethodPost, url, http.Header{"Content-Type": {"application/json"}}, body)
	require.NotNil(t, response, "nothing answers at %s", url)
	require.Equal(t, http.StatusOK, response.status, response.body)
}

// httpResponse is an answer with its body read
type httpResponse struct {
	header http.Header
	body   string
	status int
}

// fetchURL sends one request with header and returns its answer, or nil if
// nothing answers at url. It takes a *testing.T, or the *assert.CollectT of an
// EventuallyWithT condition.
func fetchURL(t require.TestingT, method, url string, header http.Header, body string) *httpResponse {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}

	req, err := http.NewRequestWithContext(context.Background(), method, url, strings.NewReader(body))
	require.NoError(t, err)

	maps.Copy(req.Header, header)

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

	jsonrpcCalls := `tx_indexer_requests_total{api="jsonrpc",operation="getBlock",status="ok"}`
	graphqlFields := `tx_indexer_requests_total{api="graphql",operation="getBlocks",status="ok"}`

	assert.Equal(t, jsonrpcCalls+" 1", sample(scrape.body, jsonrpcCalls))
	assert.Equal(t, graphqlFields+" 1", sample(scrape.body, graphqlFields))

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

// testChainID is the chain the test node serves
const testChainID = "metrics-test"

// startNode serves a chain of empty blocks up to height, over as much of a
// node's JSON-RPC as the indexer reads, until the test ends. It returns the
// node's URL.
func startNode(t *testing.T, height int64) string {
	t.Helper()

	routes := map[string]*rpcserver.RPCFunc{
		"status": rpcserver.NewRPCFunc(
			func(_ *rpctypes.Context, _ *int64) (*ctypes.ResultStatus, error) {
				return &ctypes.ResultStatus{
					NodeInfo: p2pTypes.NodeInfo{Network: testChainID},
					SyncInfo: ctypes.SyncInfo{LatestBlockHeight: height},
				}, nil
			},
			"heightGte",
		),
		"genesis": rpcserver.NewRPCFunc(
			func(_ *rpctypes.Context) (*ctypes.ResultGenesis, error) {
				return &ctypes.ResultGenesis{
					Genesis: &bftTypes.GenesisDoc{ChainID: testChainID, AppState: gnoland.GnoGenesisState{}},
				}, nil
			},
			"",
		),
		"block": rpcserver.NewRPCFunc(
			func(_ *rpctypes.Context, height *int64) (*ctypes.ResultBlock, error) {
				return &ctypes.ResultBlock{
					Block: &bftTypes.Block{Header: bftTypes.Header{ChainID: testChainID, Height: *height}},
				}, nil
			},
			"height",
		),
		"block_results": rpcserver.NewRPCFunc(
			func(_ *rpctypes.Context, height *int64) (*ctypes.ResultBlockResults, error) {
				return &ctypes.ResultBlockResults{Height: *height, Results: &state.ABCIResponses{}}, nil
			},
			"height",
		),
	}

	mux := http.NewServeMux()
	rpcserver.RegisterRPCFuncs(mux, routes, slog.New(slog.DiscardHandler))

	node := httptest.NewServer(mux)
	t.Cleanup(node.Close)

	return node.URL
}

// The metrics carry the height the fetcher read from the node and the height
// it indexed up to, so the lag behind the chain is their difference
func TestStartCfg_ServesTheFetchHeights(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, true, "--remote", startNode(t, 3))

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		scrape := get(c, "http://"+started.metricsAddress+"/metrics")
		require.NotNil(c, scrape)

		assert.Equal(c, "tx_indexer_chain_height 3", sample(scrape.body, "tx_indexer_chain_height"))
		assert.Equal(c, "tx_indexer_indexed_height 3", sample(scrape.body, "tx_indexer_indexed_height"))
	}, 5*time.Second, 10*time.Millisecond)
}

// While the node is down at start, the genesis bootstrap's failed reads of
// its status count as chain_height fetch errors: the fetcher waits on the
// bootstrap, and the lag alone would read 0
func TestStartCfg_CountsTheNodeDownAtStart(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, true)

	series := `tx_indexer_fetch_errors_total{stage="chain_height"}`

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		scrape := get(c, "http://"+started.metricsAddress+"/metrics")
		require.NotNil(c, scrape)

		failures := sample(scrape.body, series)
		assert.NotEmpty(c, failures)
		assert.NotEqual(c, series+" 0", failures)
	}, 5*time.Second, 10*time.Millisecond)
}

// Restarted on an indexed database while the node is down, the indexer
// reports the height the database holds, which it keeps serving
func TestStartCfg_ReportsTheStoredHeightWithTheNodeDown(t *testing.T) {
	t.Parallel()

	dbPath := t.TempDir()

	db, err := storage.NewPebble(dbPath, zap.NewNop())
	require.NoError(t, err)

	wb := db.WriteBatch()
	require.NoError(t, wb.SetLatestHeight(5))
	require.NoError(t, wb.Commit())
	require.NoError(t, db.Close())

	started := startIndexer(t, true, "--db-path", dbPath)

	scrape := get(t, "http://"+started.metricsAddress+"/metrics")
	require.NotNil(t, scrape)

	assert.Equal(t, "tx_indexer_indexed_height 5", sample(scrape.body, "tx_indexer_indexed_height"))
}

// TestStartCfg_RequestLogFlags makes sure the request log is off unless the
// operator turns it on, and that the client address is the address the
// request came from unless a header is trusted.
func TestStartCfg_RequestLogFlags(t *testing.T) {
	t.Parallel()

	unset := &startCfg{}
	unsetFlags := flag.NewFlagSet("start", flag.ContinueOnError)
	unset.registerFlags(unsetFlags)

	require.NoError(t, unsetFlags.Parse(nil))
	assert.False(t, unset.requestLog)
	assert.Empty(t, unset.clientIPHeader)
	assert.Equal(t, 1, unset.clientIPTrustedHops)

	set := &startCfg{}
	setFlags := flag.NewFlagSet("start", flag.ContinueOnError)
	set.registerFlags(setFlags)

	require.NoError(t, setFlags.Parse([]string{
		"--request-log",
		"--client-ip-header", "X-Forwarded-For",
		"--client-ip-trusted-hops", "2",
	}))
	assert.True(t, set.requestLog)
	assert.Equal(t, "X-Forwarded-For", set.clientIPHeader)
	assert.Equal(t, 2, set.clientIPTrustedHops)
}

// TestStartCfg_InvalidTrustedHops makes sure fewer than one trusted proxy
// fails at startup rather than leave the client address unread.
func TestStartCfg_InvalidTrustedHops(t *testing.T) {
	t.Parallel()

	cfg := &startCfg{
		clientIPTrustedHops: 0,
		dbPath:              t.TempDir(),
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorContains(t, cfg.exec(ctx), "--client-ip-trusted-hops")
}

// statuses sends url one GET per header, in turn, and returns the status each
// is answered with
func statuses(t *testing.T, url string, headers ...http.Header) []int {
	t.Helper()

	answered := make([]int, 0, len(headers))

	for _, header := range headers {
		response := fetchURL(t, http.MethodGet, url, header, "")
		require.NotNil(t, response, "nothing answers at %s", url)

		answered = append(answered, response.status)
	}

	return answered
}

// The rate limit counts a client behind two trusted proxies under the address
// the first appended to X-Forwarded-For, whichever node of the second serves
// it: what the client writes itself, left of those entries or in another
// header, never makes it a new client
func TestStartCfg_RateLimitIgnoresForgedAddresses(t *testing.T) {
	t.Parallel()

	started := startIndexer(
		t,
		false,
		"--http-rate-limit", "2",
		"--client-ip-header", "X-Forwarded-For",
		"--client-ip-trusted-hops", "2",
	)

	assert.Equal(
		t,
		[]int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests},
		statuses(
			t,
			"http://"+started.apiAddress+"/health",
			http.Header{"X-Forwarded-For": {"6.6.6.1, 203.0.113.9, 198.51.100.4"}},
			http.Header{"X-Forwarded-For": {"203.0.113.9, 198.51.100.5"}, "X-Real-Ip": {"6.6.6.2"}},
			http.Header{"X-Forwarded-For": {"203.0.113.9, 198.51.100.6"}, "True-Client-Ip": {"6.6.6.3"}},
		),
	)
}

// Two clients behind the trusted proxy are counted apart, each under the
// address the proxy names: one past the limit leaves the other served
func TestStartCfg_RateLimitCountsClientsApart(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, "--http-rate-limit", "2", "--client-ip-header", "CF-Connecting-IP")

	assert.Equal(
		t,
		[]int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests, http.StatusOK},
		statuses(
			t,
			"http://"+started.apiAddress+"/health",
			http.Header{"Cf-Connecting-Ip": {"203.0.113.9"}},
			http.Header{"Cf-Connecting-Ip": {"203.0.113.9"}},
			http.Header{"Cf-Connecting-Ip": {"203.0.113.9"}},
			http.Header{"Cf-Connecting-Ip": {"203.0.113.10"}},
		),
	)
}

// With no header trusted, the rate limit counts every request from one
// address as one client's, whatever addresses its headers name
func TestStartCfg_RateLimitCountsThePeerByDefault(t *testing.T) {
	t.Parallel()

	started := startIndexer(t, false, "--http-rate-limit", "2")

	// startIndexer's readiness probe, sent from the same address, is the
	// first request the limit counts
	assert.Equal(
		t,
		[]int{http.StatusOK, http.StatusTooManyRequests, http.StatusTooManyRequests},
		statuses(
			t,
			"http://"+started.apiAddress+"/health",
			http.Header{"X-Forwarded-For": {"6.6.6.1"}},
			http.Header{"X-Real-Ip": {"6.6.6.2"}},
			http.Header{"True-Client-Ip": {"6.6.6.3"}},
		),
	)
}

// serveCall serves one getBlock call through o, from a client behind two
// proxies appending to X-Forwarded-For
func serveCall(o *observe.Observer) {
	r := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9, 198.51.100.4")

	o.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctx, call := o.Start(r.Context(), metrics.APIJSONRPC, "getBlock")
		call.End(ctx, nil)
	})).ServeHTTP(httptest.NewRecorder(), r)
}

// The request log writes one line per request, under the requests logger,
// with the client address the trusted header names
func TestStartCfg_ObserverLogsRequests(t *testing.T) {
	t.Parallel()

	cfg := &startCfg{requestLog: true, clientIPHeader: "X-Forwarded-For", clientIPTrustedHops: 2}

	core, logs := observer.New(zap.InfoLevel)
	serveCall(cfg.newObserver(nil, zap.New(core)))

	entries := logs.All()
	require.Len(t, entries, 1)

	assert.Equal(t, "requests", entries[0].LoggerName)
	assert.Equal(t, "203.0.113.9", entries[0].ContextMap()["client"])
}

// With the request log off, the default, no request is logged, whether the
// metrics are on or not; with the metrics off too, there is nothing to observe
func TestStartCfg_ObserverLogsNothingByDefault(t *testing.T) {
	t.Parallel()

	cfg := &startCfg{clientIPHeader: "X-Forwarded-For", clientIPTrustedHops: 1}

	core, logs := observer.New(zap.InfoLevel)

	assert.Nil(t, cfg.newObserver(nil, zap.New(core)))

	o := cfg.newObserver(metrics.New(), zap.New(core))
	require.NotNil(t, o)

	serveCall(o)

	assert.Empty(t, logs.All())
}

// errSyncFailed is the error a failingSync sink fails each sync with
var errSyncFailed = errors.New("sync failed")

// failingSync is a log sink whose every sync fails, as a disk's can
type failingSync struct {
	io.Writer
}

// Sync fails with errSyncFailed
func (failingSync) Sync() error {
	return errSyncFailed
}

// failingSyncLogger is a logger whose every sync fails with errSyncFailed
func failingSyncLogger() *zap.Logger {
	return zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		failingSync{io.Discard},
		zap.InfoLevel,
	))
}

// The logger is synced, and the sync's error returned, when the stderr it
// writes to is a regular file. A pipe or a character device takes no sync, and
// is not synced, as a socket is not (start_unix_test.go). A stderr whose kind
// cannot be told is an error.
func TestSyncLogger(t *testing.T) {
	t.Parallel()

	file, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, file.Close())
	})

	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, reader.Close())
		assert.NoError(t, writer.Close())
	})

	closed, err := os.Create(filepath.Join(t.TempDir(), "closed"))
	require.NoError(t, err)
	require.NoError(t, closed.Close())

	// A character device
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, devNull.Close())
	})

	testTable := []struct {
		want error
		out  *os.File
		name string
	}{
		{errSyncFailed, file, "a regular file"},
		{nil, writer, "a pipe"},
		{nil, devNull, "/dev/null"},
		{os.ErrClosed, closed, "a closed file"},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.ErrorIs(t, syncLogger(failingSyncLogger(), testCase.out), testCase.want)
		})
	}
}

// TestStartCfg_LogFormatFlag makes sure the logs are in the console format
// unless the operator sets another.
func TestStartCfg_LogFormatFlag(t *testing.T) {
	t.Parallel()

	unset := &startCfg{}
	unsetFlags := flag.NewFlagSet("start", flag.ContinueOnError)
	unset.registerFlags(unsetFlags)

	require.NoError(t, unsetFlags.Parse(nil))
	assert.Equal(t, "console", unset.logFormat)

	set := &startCfg{}
	setFlags := flag.NewFlagSet("start", flag.ContinueOnError)
	set.registerFlags(setFlags)

	require.NoError(t, setFlags.Parse([]string{"--log-format", "json"}))
	assert.Equal(t, "json", set.logFormat)
}

// TestStartCfg_UnknownLogFormat makes sure a log format other than console or
// json fails at startup, naming the flag, rather than log in a format no
// reader expects.
func TestStartCfg_UnknownLogFormat(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorContains(t, testCfg(t, "--log-format", "xml").exec(ctx), "--log-format")
}

// The standard log package writes to stderr again once the command returns:
// its redirect ends with the logger it writes through. The test is
// sequential: the standard logger is the process's own, and every other test
// that runs the command redirects it.
func TestStartCfg_RestoresTheStandardLogger(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, taken.Close())
	})

	require.ErrorIs(t, testCfg(t, "--listen-address", taken.Addr().String()).exec(t.Context()), syscall.EADDRINUSE)

	assert.Equal(t, io.Writer(os.Stderr), log.Writer())
}
