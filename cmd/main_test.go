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
