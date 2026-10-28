package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runMainEnv names the environment variable that makes the test binary run
// the indexer instead of the tests, on the arguments it was started with
const runMainEnv = "TX_INDEXER_TEST_RUN_MAIN"

// TestMain runs the tests, or the indexer itself with runMainEnv set, so a
// test can start the command as a process of its own and read its exit code
func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) != "" {
		main()

		// main returning is a zero exit, as it is in the built binary
		os.Exit(0)
	}

	os.Exit(m.Run())
}

// startedLine is what the HTTP server logs once it listens. The command
// handles the stop signals by then: a signal sent earlier would kill it.
const startedLine = "HTTP server started"

// indexerProcess is the start command running as a process of its own, its
// stderr a pipe the test reads line by line
type indexerProcess struct {
	cmd   *exec.Cmd
	lines *bufio.Scanner

	// started is the line the HTTP server logged once it listened
	started string

	// output is every line read from stderr so far
	output strings.Builder

	// reaped reports whether stop waited for the process
	reaped bool
}

// newCommand is the start command as a process of its own, with args after
// its test defaults and stdout a regular file. It is killed once ctx ends.
func newCommand(ctx context.Context, t *testing.T, args ...string) *exec.Cmd {
	t.Helper()

	cmd := exec.CommandContext(
		ctx,
		os.Args[0],
		append([]string{
			"start",
			"--listen-address", "127.0.0.1:0",
			"--db-path", t.TempDir(),
			// No chain listens there: the genesis bootstrap retries until the
			// stop, while the API serves the empty database
			"--remote", "http://127.0.0.1:1",
		}, args...)...,
	)

	// The local zone is UTC+05:30 on every machine, so a time written in UTC
	// is one converted to it. A zone that fails to load would leave UTC: the
	// console test pins the offset, so it fails then.
	cmd.Env = append(os.Environ(), runMainEnv+"=1", "TZ=Asia/Kolkata")

	// stdout, which the logger never writes to, is a file: the stop is judged
	// on the kind of stderr alone
	stdout, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, stdout.Close())
	})

	cmd.Stdout = stdout

	return cmd
}

// runProcess runs the start command as a process of its own, with args after
// its test defaults, to its end, and returns its stderr as it wrote it and how
// it exited
func runProcess(t *testing.T, args ...string) (string, error) {
	t.Helper()

	// A command that never stops is killed, so the test ends on a failure
	// instead of hanging
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cmd := newCommand(ctx, t, args...)

	var stderr strings.Builder

	cmd.Stderr = &stderr

	err := cmd.Run()

	return stderr.String(), err
}

// startProcess runs the start command as a process of its own, with args
// after its test defaults, stdout a regular file and stderr a pipe, and
// returns once the HTTP server listens. The process is killed and reaped when
// the test ends, unless stop has reaped it.
func startProcess(t *testing.T, args ...string) *indexerProcess {
	t.Helper()

	// A command that never stops is killed, so the test ends on a failure
	// instead of hanging
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	cmd := newCommand(ctx, t, args...)

	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	p := &indexerProcess{cmd: cmd, lines: bufio.NewScanner(stderr)}

	// A failed assertion returns before stop's wait: the child is then killed
	// and reaped before the test's directories are removed
	t.Cleanup(func() {
		if p.reaped {
			return
		}

		cancel()
		t.Logf("the indexer, killed on the test's failure, stopped: %v", cmd.Wait())
	})

	p.started = p.waitFor(startedLine)

	require.NoError(t, p.lines.Err())
	require.NotEmpty(t, p.started, "the indexer never served:\n%s", p.output.String())

	return p
}

// waitFor reads stderr up to the first line holding needle and returns it,
// or returns "" once stderr ends without one
func (p *indexerProcess) waitFor(needle string) string {
	for p.lines.Scan() {
		p.output.WriteString(p.lines.Text() + "\n")

		if strings.Contains(p.lines.Text(), needle) {
			return p.lines.Text()
		}
	}

	return ""
}

// stop sends the process SIGTERM, as Docker and systemd stop it, reads its
// stderr to the end, and returns how it exited
func (p *indexerProcess) stop(t *testing.T) error {
	t.Helper()

	require.NoError(t, p.cmd.Process.Signal(syscall.SIGTERM))

	// The pipe is read to its end before the wait, which closes it
	for p.lines.Scan() {
		p.output.WriteString(p.lines.Text() + "\n")
	}

	require.NoError(t, p.lines.Err())

	p.reaped = true

	return p.cmd.Wait()
}

// A stop on SIGTERM, as Docker and systemd send it, exits 0 when stderr is a
// pipe, as both hand it to the indexer
func TestStartCfg_StopExitsZeroWithStderrAPipe(t *testing.T) {
	t.Parallel()

	p := startProcess(t)

	require.NoError(t, p.stop(t), "the indexer's stderr:\n%s", p.output.String())
}

// consoleTime matches the console format's time column: ISO 8601 to the
// millisecond, in the local zone, the one startProcess runs the indexer in
const consoleTime = `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}\+0530`

// The console format, the default, writes the columns it always has: time,
// level, logger, caller and message, then the fields as one JSON object. The
// start command's own lines are under the start logger.
func TestStartCfg_LogsTheConsoleFormatByDefault(t *testing.T) {
	t.Parallel()

	p := startProcess(t, "--request-log")
	require.NoError(t, p.stop(t), "the indexer's stderr:\n%s", p.output.String())

	assert.Regexp(
		t,
		`^`+consoleTime+`\tINFO\thttp-server\tserve/server\.go:\d+\tHTTP server started\t`+
			`\{"address": "127\.0\.0\.1:\d+"\}$`,
		p.started,
	)
	assert.Regexp(
		t,
		`(?m)^`+consoleTime+`\tINFO\tstart\tcmd/start\.go:\d+\trequest log set\t`+
			`\{"client-ip-header": "", "client-ip-trusted-hops": 1\}$`,
		p.output.String(),
	)
}

// mdbxForkNotice matches a line libmdbx, linked in through gno, prints on
// stderr on macOS, outside the indexer's logger. Its numbers are a line of
// libmdbx's source and a count, so it is matched by its shape.
var mdbxForkNotice = regexp.MustCompile(`^rthc_afterfork:\d+ drown \d+ rthc entries$`)

// startedAddress reads the address the HTTP server listens on from the line
// it logged once it listened, in either format
var startedAddress = regexp.MustCompile(`"address": ?"([^"]+)"`)

// infoAndAbove are the levels a run at info, the default, writes
var infoAndAbove = []string{"info", "warn", "error"}

// jsonLines reads output, the stderr of an indexer logging in JSON, by
// message. Each line must be one JSON object at one of levels, with its time
// in RFC 3339 with nine digits of nanoseconds, in UTC, under ts, and a logger,
// a caller and a msg.
func jsonLines(t *testing.T, output string, levels ...string) map[string][]map[string]any {
	t.Helper()

	byMessage := make(map[string][]map[string]any)

	for line := range strings.Lines(output) {
		line = strings.TrimSuffix(line, "\n")
		if mdbxForkNotice.MatchString(line) {
			continue
		}

		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry), "a line that is no JSON object: %s", line)

		ts, ok := entry["ts"].(string)
		require.True(t, ok, "no ts in %s", line)
		assert.Regexp(t, `T\d{2}:\d{2}:\d{2}\.\d{9}Z$`, ts, "ts is not to the nanosecond in UTC, in nine digits")

		_, err := time.Parse(time.RFC3339, ts)
		require.NoError(t, err)

		level, _ := entry["level"].(string)
		assert.Contains(t, levels, level, line)

		for _, key := range []string{"logger", "caller", "msg"} {
			assert.NotEmpty(t, entry[key], "no %s in %s", key, line)
		}

		msg, _ := entry["msg"].(string)
		byMessage[msg] = append(byMessage[msg], entry)
	}

	return byMessage
}

// With --log-format json, every line is one JSON object: its time in RFC 3339
// with nanoseconds, in UTC, under ts, then level, logger, caller and msg, and
// each field at the top level, the request log's included
func TestStartCfg_LogsJSONLines(t *testing.T) {
	t.Parallel()

	p := startProcess(
		t,
		"--log-format", "json",
		"--request-log",
		"--query-deadline", "10s",
		"--http-rate-limit", "100",
	)

	address := startedAddress.FindStringSubmatch(p.started)
	require.Len(t, address, 2, "no address in %q", p.started)

	post(t, "http://"+address[1]+"/", `{"jsonrpc": "2.0", "id": 1, "method": "getBlock", "params": ["1"]}`)
	post(t, "http://"+address[1]+"/graphql/query", `{"query": "{ getBlocks(where: {}) { height } }"}`)

	require.NoError(t, p.stop(t), "the indexer's stderr:\n%s", p.output.String())

	byMessage := jsonLines(t, p.output.String(), infoAndAbove...)

	for _, msg := range []string{"rate-limit set", "query deadline set", "request log set"} {
		require.Len(t, byMessage[msg], 1, msg)
		assert.Equal(t, "start", byMessage[msg][0]["logger"], msg)
	}

	// The genesis bootstrap warns on its first read of the node, which is down,
	// and a warning carries its stack trace
	warnings := byMessage["unable to bootstrap genesis, retrying"]
	require.NotEmpty(t, warnings)
	assert.Contains(t, warnings[0], "stacktrace")

	served := byMessage["served request"]
	require.Len(t, served, 2)

	for _, entry := range served {
		assert.Equal(t, "requests", entry["logger"])

		for _, key := range []string{"api", "operation", "status", "duration", "rows-scanned", "rows-returned", "client"} {
			assert.Contains(t, entry, key)
		}
	}

	assert.ElementsMatch(t, []any{"jsonrpc", "graphql"}, []any{served[0]["api"], served[1]["api"]})
}

// webSocketHandshake is the header of a WebSocket upgrade gorilla accepts
// from a client of the indexer's own origin
func webSocketHandshake() http.Header {
	return http.Header{
		"Connection":            {"Upgrade"},
		"Upgrade":               {"websocket"},
		"Sec-Websocket-Version": {"13"},
		"Sec-Websocket-Key":     {"dGhlIHNhbXBsZSBub25jZQ=="},
	}
}

// crossOrigin is a WebSocket handshake from another origin, which gorilla
// refuses
func crossOrigin() http.Header {
	header := webSocketHandshake()
	header.Set("Origin", "http://elsewhere.example")

	return header
}

// failedUpgrades are requests gqlgen's WebSocket transport takes, by their
// Upgrade header, and fails to upgrade, which any client may send, with the
// status gorilla refuses each with: curl --http2's h2c upgrade, a query
// posted with a WebSocket's header, a handshake from another origin, and one
// posted
var failedUpgrades = []struct {
	header http.Header
	method string
	body   string
	status int
}{
	{
		http.Header{
			"Connection":     {"Upgrade, HTTP2-Settings"},
			"Upgrade":        {"h2c"},
			"Http2-Settings": {"AAMAAABkAAQCAAAAAAIAAAAA"},
		},
		http.MethodGet,
		"",
		http.StatusBadRequest,
	},
	{
		http.Header{"Upgrade": {"websocket"}, "Content-Type": {"application/json"}},
		http.MethodPost,
		`{"query": "{ latestBlockHeight }"}`,
		http.StatusBadRequest,
	},
	{crossOrigin(), http.MethodGet, "", http.StatusForbidden},
	{webSocketHandshake(), http.MethodPost, "", http.StatusMethodNotAllowed},
}

// sendWithHeader sends one request with header and returns its answer
func sendWithHeader(t *testing.T, method, url, body string, header http.Header) httpResponse {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	require.NoError(t, err)

	req.Header = header

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer func() {
		assert.NoError(t, resp.Body.Close())
	}()

	read, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return httpResponse{header: resp.Header, status: resp.StatusCode, body: string(read)}
}

// gorillaEarlyData is the text of gorilla's error for an upgrade whose client
// sent data before the handshake completed, which gqlgen's line of the failed
// upgrade holds
const gorillaEarlyData = "client sent data before handshake is complete"

// upgradeWithTrailingByte is a valid GraphQL WebSocket upgrade to host with
// one byte after it in the same write, which any client may send: gorilla
// fails it once it has taken the connection from the HTTP server, and closes
// the connection unanswered
func upgradeWithTrailingByte(host string) string {
	return "GET /graphql/query HTTP/1.1\r\nHost: " + host + "\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Protocol: graphql-transport-ws\r\n\r\nX"
}

// A request gqlgen's WebSocket transport fails to upgrade is answered gqlgen's
// body under gorilla's status, naming the WebSocket version it speaks, or
// nothing once gorilla has taken the connection, and writes its line, through
// the standard log package, as a line of the logger at warn under stdlib:
// with --log-format json, every line is still one JSON object, with no stack
// trace, and none is at error, as no client alone causes one
func TestStartCfg_LogsAFailedGraphQLUpgradeAtWarn(t *testing.T) {
	t.Parallel()

	p := startProcess(t, "--log-format", "json")

	address := startedAddress.FindStringSubmatch(p.started)
	require.Len(t, address, 2, "no address in %q", p.started)

	for _, upgrade := range failedUpgrades {
		response := sendWithHeader(t, upgrade.method, "http://"+address[1]+"/graphql/query", upgrade.body, upgrade.header)
		assert.Equal(t, upgrade.status, response.status, response.body)
		assert.JSONEq(t, `{"errors": [{"message": "unable to upgrade"}], "data": null}`, response.body)
		// The version a WebSocket client is to retry with, as RFC 6455 §4.4 has
		// a server name it
		assert.Equal(t, "13", response.header.Get("Sec-Websocket-Version"))
	}

	conn, err := net.Dial("tcp", address[1])
	require.NoError(t, err)

	_, err = io.WriteString(conn, upgradeWithTrailingByte(address[1]))
	require.NoError(t, err)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))

	answer, err := io.ReadAll(conn)
	require.NoError(t, err)
	assert.Empty(t, answer)
	require.NoError(t, conn.Close())

	// gqlgen logs the failure once gorilla has closed the connection
	require.NotEmpty(t, p.waitFor(gorillaEarlyData), p.output.String())

	require.NoError(t, p.stop(t), "the indexer's stderr:\n%s", p.output.String())

	var stdlib []map[string]any

	for _, entries := range jsonLines(t, p.output.String(), "info", "warn") {
		for _, entry := range entries {
			if entry["logger"] == "stdlib" {
				stdlib = append(stdlib, entry)
			}
		}
	}

	require.Len(t, stdlib, len(failedUpgrades)+1, p.output.String())

	for _, entry := range stdlib {
		assert.Equal(t, "warn", entry["level"])
		assert.Contains(t, entry["msg"], "unable to upgrade")
		assert.Contains(t, entry["caller"], "transport/websocket.go")
		// Any client causes one: its stack would be a page of log per request
		assert.NotContains(t, entry, "stacktrace")
	}
}

// The error that ends a run once the logger exists is logged, at error, under
// the start logger, in the run's format, and printed nowhere else: here, an
// address another listener holds
func TestStartCfg_LogsTheErrorThatEndsARun(t *testing.T) {
	t.Parallel()

	taken, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, taken.Close())
	})

	stderr, err := runProcess(t, "--log-format", "json", "--listen-address", taken.Addr().String())

	var exited *exec.ExitError

	require.ErrorAs(t, err, &exited)
	assert.Equal(t, 1, exited.ExitCode())

	stopped := jsonLines(t, stderr, infoAndAbove...)["stopped on an error"]
	require.Len(t, stopped, 1, stderr)

	assert.Equal(t, "error", stopped[0]["level"])
	assert.Equal(t, "start", stopped[0]["logger"])
	assert.Contains(t, stopped[0]["error"], "address already in use")
}

// An error before the logger exists is printed plain on stderr, on a line of
// its own: here, a log level that does not parse
func TestStartCfg_PrintsAnErrorBeforeTheLogger(t *testing.T) {
	t.Parallel()

	stderr, err := runProcess(t, "--log-level", "nonsense")

	var exited *exec.ExitError

	require.ErrorAs(t, err, &exited)
	assert.Equal(t, 1, exited.ExitCode())

	assert.Regexp(t, `^unable to parse log level, .*nonsense.*\n$`, withoutForkNotice(stderr))
}

// withoutForkNotice is stderr without libmdbx's fork notice
func withoutForkNotice(stderr string) string {
	var kept strings.Builder

	for line := range strings.Lines(stderr) {
		if !mdbxForkNotice.MatchString(strings.TrimSuffix(line, "\n")) {
			kept.WriteString(line)
		}
	}

	return kept.String()
}

// At a level above error, the error that ends a run is not logged: it is
// printed plain on stderr, on a line of its own, as an error before the
// logger exists is
func TestStartCfg_PrintsTheErrorThatEndsARunAboveTheErrorLevel(t *testing.T) {
	t.Parallel()

	taken, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, taken.Close())
	})

	stderr, err := runProcess(t, "--log-level", "fatal", "--listen-address", taken.Addr().String())

	var exited *exec.ExitError

	require.ErrorAs(t, err, &exited)
	assert.Equal(t, 1, exited.ExitCode())

	assert.Regexp(t, `^[^\n]*address already in use[^\n]*\n$`, withoutForkNotice(stderr))
}
