package model

import (
	"testing"

	"github.com/gnolang/gno/gno.land/pkg/sdk/vm"
	"github.com/gnolang/gno/gnovm/stdlibs/chain"
	"github.com/gnolang/gno/tm2/pkg/crypto"
	"github.com/gnolang/gno/tm2/pkg/sdk/bank"
	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/stretchr/testify/require"
)

const testPkgPath = "gno.land/r/demo/foo"

var (
	testFrom = crypto.AddressFromPreimage([]byte("from"))
	testTo   = crypto.AddressFromPreimage([]byte("to"))
)

func TestMakeEvent_TransferEvent(t *testing.T) {
	t.Parallel()

	event, err := makeEvent(bank.TransferEvent{
		From:  testFrom.String(),
		To:    testTo.String(),
		Coins: std.NewCoins(std.NewCoin("ugnot", 1000000)),
	})
	require.NoError(t, err)

	require.Equal(t, &TransferEvent{
		Type:  "TransferEvent",
		From:  testFrom.String(),
		To:    testTo.String(),
		Coins: "1000000ugnot",
	}, event)
}

func TestMakeEvent_StorageUnlockEventRefundWithheld(t *testing.T) {
	t.Parallel()

	event, err := makeEvent(chain.StorageUnlockEvent{
		BytesDelta:     -64,
		FeeRefund:      std.NewCoin("ugnot", 6400),
		PkgPath:        testPkgPath,
		RefundWithheld: true,
	})
	require.NoError(t, err)

	require.Equal(t, &StorageUnlockEvent{
		Type:           "StorageUnlockEvent",
		BytesDelta:     -64,
		FeeRefund:      &Coin{Amount: 6400, Denom: "ugnot"},
		PkgPath:        testPkgPath,
		RefundWithheld: true,
	}, event)
}

func TestNewTransactionMessage_EnablePackage(t *testing.T) {
	t.Parallel()

	message := NewTransactionMessage(vm.MsgEnablePackage{
		Approver:  testFrom,
		PkgPath:   testPkgPath,
		PkgHash:   "abc123",
		PkgHeight: 42,
	})

	require.Equal(t, &TransactionMessage{
		Route:   MessageRouteVM.String(),
		TypeURL: MessageTypeEnablePackage.String(),
		Value: MsgEnablePackage{
			Approver:  testFrom.String(),
			PkgPath:   testPkgPath,
			PkgHash:   "abc123",
			PkgHeight: 42,
		},
	}, message)
}

func TestMakeVMMsgCall_ArgsFilter(t *testing.T) {
	t.Parallel()

	msg := makeVMMsgCall(vm.MsgCall{
		Caller:  testFrom,
		PkgPath: testPkgPath,
		Func:    "Transfer",
		Args:    []string{testTo.String(), "1000"},
	})

	require.Equal(t, []*IndexedString{
		{Index: 0, Value: testTo.String()},
		{Index: 1, Value: "1000"},
	}, msg.ArgsAt)

	argAt := func(index int, value string) *NestedFilterIndexedString {
		return &NestedFilterIndexedString{
			Index: &FilterInt{Eq: &index},
			Value: &FilterString{Eq: &value},
		}
	}

	amount, missing := "1000", "2000"

	testTable := []struct {
		filter   *NestedFilterMsgCall
		name     string
		expected bool
	}{
		{
			name:     "value at matching position",
			filter:   &NestedFilterMsgCall{ArgsAt: argAt(1, "1000")},
			expected: true,
		},
		{
			name:     "value at other position",
			filter:   &NestedFilterMsgCall{ArgsAt: argAt(0, "1000")},
			expected: false,
		},
		{
			name: "several positions",
			filter: &NestedFilterMsgCall{And: []*NestedFilterMsgCall{
				{ArgsAt: argAt(0, testTo.String())},
				{ArgsAt: argAt(1, "1000")},
			}},
			expected: true,
		},
		{
			name:     "position out of range",
			filter:   &NestedFilterMsgCall{ArgsAt: argAt(2, "1000")},
			expected: false,
		},
		{
			name:     "args matches any position",
			filter:   &NestedFilterMsgCall{Args: &FilterString{Eq: &amount}},
			expected: true,
		},
		{
			name:     "args with no matching value",
			filter:   &NestedFilterMsgCall{Args: &FilterString{Eq: &missing}},
			expected: false,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, testCase.expected, testCase.filter.Eval(&msg))
		})
	}
}

func TestNewTransactionMessage_RejectPackage(t *testing.T) {
	t.Parallel()

	message := NewTransactionMessage(vm.MsgRejectPackage{
		Sender:  testFrom,
		PkgPath: testPkgPath,
	})

	require.Equal(t, &TransactionMessage{
		Route:   MessageRouteVM.String(),
		TypeURL: MessageTypeRejectPackage.String(),
		Value: MsgRejectPackage{
			Sender:  testFrom.String(),
			PkgPath: testPkgPath,
		},
	}, message)
}
