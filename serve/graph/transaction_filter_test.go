package graph

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/serve/graph/model"
)

const (
	testDenom               = "ugnot"
	testPkgPath             = "gno.land/r/demo/foo"
	storageDepositEventType = "StorageDepositEvent"
	storageUnlockEventType  = "StorageUnlockEvent"
	transferEventType       = "TransferEvent"
)

func strPtr(s string) *string { return &s }

func intPtr(n int) *int { return &n }

func boolPtr(b bool) *bool { return &b }

func TestFilteredEventBy(t *testing.T) {
	t.Parallel()

	gno := &model.GnoEvent{
		Type:    "Transfer",
		PkgPath: testPkgPath,
		Attrs:   []*model.GnoEventAttribute{{Key: "to", Value: "g1abc"}},
	}
	deposit := &model.StorageDepositEvent{
		Type:       storageDepositEventType,
		BytesDelta: 128,
		FeeDelta:   &model.Coin{Amount: 12800, Denom: testDenom},
		PkgPath:    testPkgPath,
	}
	unlock := &model.StorageUnlockEvent{
		Type:       storageUnlockEventType,
		BytesDelta: -64,
		FeeRefund:  &model.Coin{Amount: 6400, Denom: testDenom},
		PkgPath:    testPkgPath,
	}
	withheld := &model.StorageUnlockEvent{Type: storageUnlockEventType, PkgPath: testPkgPath, RefundWithheld: true}
	feeless := &model.StorageDepositEvent{Type: storageDepositEventType, PkgPath: testPkgPath}
	unknown := &model.UnknownEvent{Value: "{}"}

	cases := []struct {
		event model.Event
		input *model.EventInput
		name  string
		want  bool
	}{
		{nil, &model.EventInput{}, "nil event never matches", false},
		{deposit, &model.EventInput{}, "no sub-input matches any event", true},
		{gno, &model.EventInput{}, "no sub-input matches a gno event as well", true},
		{
			gno,
			&model.EventInput{GnoEvent: &model.GnoEventInput{Type: strPtr("Transfer"), PkgPath: strPtr(testPkgPath)}},
			"gno event by type and pkg path",
			true,
		},
		{
			gno,
			&model.EventInput{GnoEvent: &model.GnoEventInput{Type: strPtr("Mint")}},
			"gno event with another type",
			false,
		},
		{
			gno,
			&model.EventInput{GnoEvent: &model.GnoEventInput{
				Attrs: []*model.EventAttributeInput{{Key: strPtr("to"), Value: strPtr("g1abc")}},
			}},
			"gno event by attribute",
			true,
		},
		{deposit, &model.EventInput{GnoEvent: &model.GnoEventInput{}}, "gno input against a storage event", false},
		{
			deposit,
			&model.EventInput{StorageDepositEvent: &model.StorageDepositEventInput{
				BytesDelta: intPtr(128),
				FeeDelta:   &model.CoinInput{Amount: intPtr(12800), Denom: strPtr(testDenom)},
			}},
			"storage deposit by bytes and fee",
			true,
		},
		{
			deposit,
			&model.EventInput{StorageDepositEvent: &model.StorageDepositEventInput{
				FeeDelta: &model.CoinInput{Amount: intPtr(1)},
			}},
			"storage deposit with another fee",
			false,
		},
		{
			unlock,
			&model.EventInput{StorageUnlockEvent: &model.StorageUnlockEventInput{PkgPath: strPtr(testPkgPath)}},
			"storage unlock by pkg path",
			true,
		},
		{
			deposit,
			&model.EventInput{StorageUnlockEvent: &model.StorageUnlockEventInput{}},
			"storage unlock input against a deposit",
			false,
		},
		{
			withheld,
			&model.EventInput{StorageUnlockEvent: &model.StorageUnlockEventInput{RefundWithheld: boolPtr(true)}},
			"storage unlock by refund withheld",
			true,
		},
		{
			unlock,
			&model.EventInput{StorageUnlockEvent: &model.StorageUnlockEventInput{RefundWithheld: boolPtr(true)}},
			"storage unlock without the refund withheld",
			false,
		},
		{
			feeless,
			&model.EventInput{StorageDepositEvent: &model.StorageDepositEventInput{
				FeeDelta: &model.CoinInput{Amount: intPtr(1)},
			}},
			"fee input against a deposit without a fee",
			false,
		},
		{unknown, &model.EventInput{}, "no sub-input matches an unknown event", true},
		{unknown, &model.EventInput{GnoEvent: &model.GnoEventInput{}}, "typed input against an unknown event", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, filteredEventBy(tc.event, tc.input))
		})
	}
}

func TestFilteredEventBy_TransferEvent(t *testing.T) {
	t.Parallel()

	transfer := &model.TransferEvent{Type: transferEventType, From: "g1from", To: "g1to", Coins: "1000000ugnot"}

	cases := []struct {
		input *model.EventInput
		name  string
		want  bool
	}{
		{&model.EventInput{}, "no sub-input matches a transfer", true},
		{
			&model.EventInput{TransferEvent: &model.TransferEventInput{From: strPtr("g1from"), To: strPtr("g1to")}},
			"transfer by from and to",
			true,
		},
		{
			&model.EventInput{TransferEvent: &model.TransferEventInput{To: strPtr("g1other")}},
			"transfer to another address",
			false,
		},
		{
			&model.EventInput{TransferEvent: &model.TransferEventInput{
				Coins: &model.AmountInput{From: intPtr(1), To: intPtr(2_000_000), Denomination: strPtr(testDenom)},
			}},
			"transfer by coin range",
			true,
		},
		{
			&model.EventInput{TransferEvent: &model.TransferEventInput{
				Coins: &model.AmountInput{From: intPtr(2_000_000), Denomination: strPtr(testDenom)},
			}},
			"transfer below the coin range",
			false,
		},
		{&model.EventInput{GnoEvent: &model.GnoEventInput{}}, "gno input against a transfer", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, filteredEventBy(transfer, tc.input))
		})
	}
}

func TestFilteredTransactionMessageBy_PackageApproval(t *testing.T) {
	t.Parallel()

	enable := &model.TransactionMessage{
		Route:   model.MessageRouteVM.String(),
		TypeURL: model.MessageTypeEnablePackage.String(),
		Value:   model.MsgEnablePackage{Approver: "g1approver", PkgPath: testPkgPath, PkgHash: "abc123", PkgHeight: 42},
	}
	reject := &model.TransactionMessage{
		Route:   model.MessageRouteVM.String(),
		TypeURL: model.MessageTypeRejectPackage.String(),
		Value:   model.MsgRejectPackage{Sender: "g1creator", PkgPath: testPkgPath},
	}

	vmParam := func(input *model.TransactionVMMessageInput) *model.TransactionMessageInput {
		return &model.TransactionMessageInput{VMParam: input}
	}

	cases := []struct {
		message *model.TransactionMessage
		input   *model.TransactionMessageInput
		name    string
		want    bool
	}{
		{
			enable,
			vmParam(&model.TransactionVMMessageInput{
				EnablePackage: &model.MsgEnablePackageInput{Approver: strPtr("g1approver"), PkgHeight: intPtr(42)},
			}),
			"enable by approver and height",
			true,
		},
		{
			enable,
			vmParam(&model.TransactionVMMessageInput{EnablePackage: &model.MsgEnablePackageInput{PkgHash: strPtr("other")}}),
			"enable with another hash",
			false,
		},
		{
			enable,
			vmParam(&model.TransactionVMMessageInput{Exec: &model.MsgCallInput{}}),
			"exec input against an enable",
			false,
		},
		{
			reject,
			vmParam(&model.TransactionVMMessageInput{
				RejectPackage: &model.MsgRejectPackageInput{Sender: strPtr("g1creator"), PkgPath: strPtr(testPkgPath)},
			}),
			"reject by sender and path",
			true,
		},
		{
			reject,
			vmParam(&model.TransactionVMMessageInput{RejectPackage: &model.MsgRejectPackageInput{Sender: strPtr("g1other")}}),
			"reject by another sender",
			false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, filteredTransactionMessageBy(tc.message, tc.input))
		})
	}
}

// A transfer of several coins renders them comma-separated, the form the
// amount filter parses, so a range on one denom still matches.
func TestFilteredEventBy_TransferEventSeveralCoins(t *testing.T) {
	t.Parallel()

	transfer := &model.TransferEvent{Type: transferEventType, From: "g1from", To: "g1to", Coins: "10foo,5ugnot"}

	require.True(t, filteredEventBy(transfer, &model.EventInput{TransferEvent: &model.TransferEventInput{
		Coins: &model.AmountInput{From: intPtr(5), To: intPtr(5), Denomination: strPtr(testDenom)},
	}}))
	require.False(t, filteredEventBy(transfer, &model.EventInput{TransferEvent: &model.TransferEventInput{
		Coins: &model.AmountInput{From: intPtr(6), Denomination: strPtr(testDenom)},
	}}))
}

// Every event type on EventInput must be dispatched by filteredEventBy and
// counted by eventInputNamesType; a new field that is not would make an input
// naming only that type match every event.
func TestEventInputFieldsAreDispatched(t *testing.T) {
	t.Parallel()

	require.Equal(t, 4, reflect.TypeOf(model.EventInput{}).NumField(),
		"EventInput gained a field: extend eventInputNamesType and filteredEventBy, then update this count")
}
