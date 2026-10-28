package main

import (
	"bufio"
	"context"
	"encoding/json"
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

	// The local zone is one away from UTC on every machine, so a time
	// written in UTC is one converted to it
	cmd.Env = append(os.Environ(), runMainEnv+"=1", "TZ=Asia/Kolkata")

	// stdout, which the logger never writes to, is a file: the stop is judged
	// on the kind of stderr alone
	stdout, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, stdout.Close())
	})

	cmd.Stdout = stdout

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

	for p.started == "" && p.lines.Scan() {
		p.output.WriteString(p.lines.Text() + "\n")

		if strings.Contains(p.lines.Text(), startedLine) {
			p.started = p.lines.Text()
		}
	}

	require.NoError(t, p.lines.Err())
	require.NotEmpty(t, p.started, "the indexer never served:\n%s", p.output.String())

	return p
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
// millisecond, in the local zone
const consoleTime = `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}(Z|[+-]\d{4})`

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

// mdbxForkNotice is a line libmdbx, linked in through gno, prints on stderr
// on macOS, outside the indexer's logger
const mdbxForkNotice = "rthc_afterfork:35891 drown 0 rthc entries"

// startedAddress reads the address the HTTP server listens on from the line
// it logged once it listened, in either format
var startedAddress = regexp.MustCompile(`"address": ?"([^"]+)"`)

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

	byMessage := make(map[string][]map[string]any)

	for line := range strings.Lines(p.output.String()) {
		line = strings.TrimSuffix(line, "\n")
		if line == mdbxForkNotice {
			continue
		}

		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry), "a line that is no JSON object: %s", line)

		ts, ok := entry["ts"].(string)
		require.True(t, ok, "no ts in %s", line)
		assert.Regexp(t, `T\d{2}:\d{2}:\d{2}\.\d+Z$`, ts, "ts is not to the nanosecond in UTC")

		_, err := time.Parse(time.RFC3339Nano, ts)
		require.NoError(t, err)

		assert.Contains(t, []any{"debug", "info", "warn", "error"}, entry["level"], line)

		for _, key := range []string{"logger", "caller", "msg"} {
			assert.NotEmpty(t, entry[key], "no %s in %s", key, line)
		}

		msg, _ := entry["msg"].(string)
		byMessage[msg] = append(byMessage[msg], entry)
	}

	for _, msg := range []string{"rate-limit set", "query deadline set", "request log set"} {
		require.Len(t, byMessage[msg], 1, msg)
		assert.Equal(t, "start", byMessage[msg][0]["logger"], msg)
	}

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
