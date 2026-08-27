package model

import (
	"testing"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/stretchr/testify/require"
)

// A transaction whose payload cannot be decoded must be distinguishable from one
// that genuinely carries nothing.
//
// getStdTx set t.stdTx = nil on a decode error and then overwrote it
// unconditionally with a pointer to the zero std.Tx, so the nil branch was dead:
// an undecodable transaction reported a zero gas fee, an empty memo and no
// messages as though those were facts about the chain, and the nil checks in
// Memo and GasFee could never fire.
func TestUndecodableTransaction(t *testing.T) {
	t.Parallel()

	tx := NewTransaction(&types.TxResult{
		Height: 1,
		Tx:     []byte{0xff, 0xfe, 0xfd, 0x00, 0x01},
	})

	require.Nil(t, tx.GasFee(), "an undecodable payload must not report a fee")
	require.Empty(t, tx.Memo())
	// Must not panic: getMessages dereferenced the result of getStdTx without a
	// nil check, which only became reachable once the nil branch worked.
	require.Empty(t, tx.Messages())
}

func TestDecodableTransactionIsUnaffected(t *testing.T) {
	t.Parallel()

	encoded, err := amino.Marshal(&std.Tx{
		Fee:  std.Fee{GasFee: std.Coin{Denom: "ugnot", Amount: 42}},
		Memo: "hello",
	})
	require.NoError(t, err)

	tx := NewTransaction(&types.TxResult{Height: 1, Tx: encoded})

	fee := tx.GasFee()
	require.NotNil(t, fee)
	require.Equal(t, 42, fee.Amount)
	require.Equal(t, "ugnot", fee.Denom)
	require.Equal(t, "hello", tx.Memo())
}
