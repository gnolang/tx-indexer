package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

// A stop on SIGTERM, as Docker and systemd send it, exits 0 when stderr is a
// pipe, as both hand it to the indexer
func TestStartCfg_StopExitsZeroWithStderrAPipe(t *testing.T) {
	t.Parallel()

	// A command that never stops is killed, so the test ends on a failure
	// instead of hanging
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		os.Args[0],
		"start",
		"--listen-address", "127.0.0.1:0",
		"--db-path", t.TempDir(),
		// No chain listens there: the genesis bootstrap retries until the
		// stop, while the API serves the empty database
		"--remote", "http://127.0.0.1:1",
	)

	cmd.Env = append(os.Environ(), runMainEnv+"=1")

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

	var (
		output  strings.Builder
		lines   = bufio.NewScanner(stderr)
		started bool
	)

	for !started && lines.Scan() {
		output.WriteString(lines.Text() + "\n")
		started = strings.Contains(lines.Text(), startedLine)
	}

	if started {
		require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	}

	// The pipe is read to its end before the wait, which closes it
	for lines.Scan() {
		output.WriteString(lines.Text() + "\n")
	}

	require.NoError(t, lines.Err())
	require.True(t, started, "the indexer never served:\n%s", output.String())
	require.NoError(t, cmd.Wait(), "the indexer's stderr:\n%s", output.String())
}
