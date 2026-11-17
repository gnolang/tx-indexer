package model

import (
	"encoding/hex"
	"testing"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/stretchr/testify/require"
)

// A MsgRun signed and encoded by gno v1.8.0, which adds MsgRun.MaxSend
// (gnolang/gno#179): the coins the run may send out of its caller's account,
// required from v1.8.0 on for any run that sends coins. It is the canonical
// encoding v1.8.0 accepts, with MaxSend "1000ugnot" as its trailing field 5.
//
// amino refuses a field number its struct does not declare, so an indexer
// built on a gno without MaxSend cannot decode this transaction at all: it
// reports it with no messages, no memo and no fee.
const msgRunWithMaxSendHex = "0ab1010a092f766d2e6d5f72756e12a3010a28673138726b6630667a7534707278776b386b7a6534" +
	"6a6c35707a327674716e393471337972387a6d226c0a046d61696e1237676e6f2e6c616e642f652f" +
	"673138726b6630667a7534707278776b386b7a65346a6c35707a327674716e393471337972387a6d" +
	"2f72756e1a2b0a0a7363726970742e676e6f121d7061636b616765206d61696e0a0a66756e63206d" +
	"61696e2829207b7d0a2a093130303075676e6f741210088092f40112093230303075676e6f741a7e" +
	"0a3a0a132f746d2e5075624b6579536563703235366b3112230a21020b272120623cacd8ddcd2317" +
	"4d62af4a4dfa68d069873c8065218410c4ce38aa12405c8f1444ee7e39ea361c0238c771fbfcf316" +
	"b09244029b3c2cf4384a21a0a0737ddde8de03934d1f97cf1710625bfaaffb71a49696dedf942784" +
	"c43396c0ba6a22106d61785f73656e642066697874757265"

const msgRunWithMaxSendCaller = "g18rkf0fzu4prxwk8kze4jl5pz2vtqn94q3yr8zm"

func msgRunWithMaxSend(t *testing.T) *Transaction {
	t.Helper()

	bz, err := hex.DecodeString(msgRunWithMaxSendHex)
	require.NoError(t, err)

	return NewTransaction(&types.TxResult{Height: 1, Tx: bz})
}

func TestTransaction_DecodesMsgRunWithMaxSend(t *testing.T) {
	t.Parallel()

	tx := msgRunWithMaxSend(t)

	require.Equal(t, "max_send fixture", tx.Memo())

	fee := tx.GasFee()
	require.NotNil(t, fee, "the transaction did not decode")
	require.Equal(t, 2000, fee.Amount)

	messages := tx.Messages()
	require.Len(t, messages, 1)

	run := messages[0].VMMsgRun()
	require.Equal(t, msgRunWithMaxSendCaller, run.Caller)
	require.Equal(t, "gno.land/e/"+msgRunWithMaxSendCaller+"/run", run.Package.Path)
}
