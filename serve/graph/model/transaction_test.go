package model

import (
	"testing"

	"github.com/gnolang/gno/gno.land/pkg/sdk/vm"
	"github.com/gnolang/gno/tm2/pkg/crypto"
	"github.com/gnolang/gno/tm2/pkg/sdk/bank"
	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTransactionMessage_EnablePackage(t *testing.T) {
	t.Parallel()

	approver := crypto.MustAddressFromString("g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5")

	tm := NewTransactionMessage(vm.MsgEnablePackage{
		Approver:  approver,
		PkgPath:   "gno.land/r/demo/foo",
		PkgHash:   "abc123",
		PkgHeight: 42,
	})

	assert.Equal(t, MessageRouteVM.String(), tm.Route)
	assert.Equal(t, MessageTypeEnablePackage.String(), tm.TypeURL)
	assert.Equal(t, MsgEnablePackage{
		Approver:  approver.String(),
		PkgPath:   "gno.land/r/demo/foo",
		PkgHash:   "abc123",
		PkgHeight: 42,
	}, tm.VMMsgEnablePackage())
}

func TestNewTransactionMessage_RejectPackage(t *testing.T) {
	t.Parallel()

	sender := crypto.MustAddressFromString("g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5")

	tm := NewTransactionMessage(vm.MsgRejectPackage{
		Sender:  sender,
		PkgPath: "gno.land/r/demo/foo",
	})

	assert.Equal(t, MessageRouteVM.String(), tm.Route)
	assert.Equal(t, MessageTypeRejectPackage.String(), tm.TypeURL)
	assert.Equal(t, MsgRejectPackage{
		Sender:  sender.String(),
		PkgPath: "gno.land/r/demo/foo",
	}, tm.VMMsgRejectPackage())
}

func TestMakeEvent_TransferEvent(t *testing.T) {
	t.Parallel()

	event, err := makeEvent(bank.TransferEvent{
		From:  "g1from",
		To:    "g1to",
		Coins: std.NewCoins(std.NewCoin("ugnot", 1000)),
	})
	require.NoError(t, err)

	transferEvent, ok := event.(*TransferEvent)
	require.True(t, ok)

	assert.Equal(t, &TransferEvent{
		Type:  "TransferEvent",
		From:  "g1from",
		To:    "g1to",
		Coins: "1000ugnot",
	}, transferEvent)
}
