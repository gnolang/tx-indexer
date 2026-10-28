//go:build unix

package main

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A socket, as systemd hands the indexer its journal's, takes no sync, and is
// not synced. The test is unix's alone, as syscall.Socketpair is.
func TestSyncLogger_LeavesASocketUnsynced(t *testing.T) {
	t.Parallel()

	sockets, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	require.NoError(t, err)

	socket, peer := os.NewFile(uintptr(sockets[0]), "socket"), os.NewFile(uintptr(sockets[1]), "peer")

	t.Cleanup(func() {
		assert.NoError(t, socket.Close())
		assert.NoError(t, peer.Close())
	})

	assert.NoError(t, syncLogger(failingSyncLogger(), socket))
}
