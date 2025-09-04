package graph

import (
	"fmt"
	"testing"

	gnostd "github.com/gnolang/gno/gnovm/stdlibs/std"
	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/gnolang/tx-indexer/serve/graph/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventsFilters(t *testing.T) {
	t.Parallel()

	txs := []*model.Transaction{
		model.NewTransaction(&types.TxResult{
			Response: abci.ResponseDeliverTx{
				ResponseBase: abci.ResponseBase{
					Events: []abci.Event{},
				},
			},
		},
		),
		model.NewTransaction(&types.TxResult{
			Response: abci.ResponseDeliverTx{
				ResponseBase: abci.ResponseBase{
					Events: []abci.Event{
						gnostd.GnoEvent{
							Type: "Registered",
							Attributes: []gnostd.GnoEventAttribute{
								{Key: "name", Value: "gno"},
							},
							PkgPath: "gno.land/r/sys/users",
						},
					},
				},
			},
		}),
		model.NewTransaction(&types.TxResult{
			Response: abci.ResponseDeliverTx{
				ResponseBase: abci.ResponseBase{
					Events: []abci.Event{
						gnostd.StorageDepositEvent{
							BytesDelta: 100,
							FeeDelta:   std.Coin{Denom: "ugno", Amount: 10},
							PkgPath:    "gno.land/r/gnoland/users/v1",
						},
					},
				},
			},
		}),
		model.NewTransaction(&types.TxResult{
			Response: abci.ResponseDeliverTx{
				ResponseBase: abci.ResponseBase{
					Events: []abci.Event{
						gnostd.StorageUnlockEvent{
							BytesDelta: 100,
							FeeRefund:  std.Coin{Denom: "ugno", Amount: 10},
							PkgPath:    "gno.land/r/gnoland/users/v1",
						},
					},
				},
			},
		}),
		model.NewTransaction(&types.TxResult{
			Response: abci.ResponseDeliverTx{
				ResponseBase: abci.ResponseBase{
					Events: []abci.Event{
						gnostd.GnoEvent{
							Type: "Registered",
							Attributes: []gnostd.GnoEventAttribute{
								{Key: "name", Value: "gno"},
							},
							PkgPath: "gno.land/r/sys/users",
						},
						gnostd.StorageDepositEvent{
							BytesDelta: 100,
							FeeDelta:   std.Coin{Denom: "ugno", Amount: 10},
							PkgPath:    "gno.land/r/gnoland/users/v1",
						},
						gnostd.StorageUnlockEvent{
							BytesDelta: 100,
							FeeRefund:  std.Coin{Denom: "ugno", Amount: 10},
							PkgPath:    "gno.land/r/gnoland/users/v1",
						},
					},
				},
			},
		}),
	}

	// Define some common filter values to reuse.
	badString := "Bad String"
	badInt := 999
	ugno := "ugno"
	gnoEventType := "Registered"
	gnoEventPkgPath := "gno.land/r/sys/users"
	gnoEventAttributeKey := "name"
	gnoEventAttributeValue := "gno"
	storageDepositeType := "StorageDepositEvent"
	storageDepositeBytesDelta := 100
	storageDepositeFeeDeltaAmount := 10
	storageDepositePkgPath := "gno.land/r/gnoland/users/v1"
	storageUnlockType := "StorageUnlockEvent"
	storageUnlockBytesDelta := 100
	storageUnlockFeeRefundAmount := 10
	storageUnlockPkgPath := "gno.land/r/gnoland/users/v1"

	tests := []struct {
		name     string
		filter   model.TransactionFilter
		expected []*model.Transaction
	}{
		{
			name:     "no filter",
			filter:   model.TransactionFilter{},
			expected: txs,
		},
		{
			name: "filter GnoEvent by event type",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						GnoEvent: &model.GnoEventInput{
							Type: &gnoEventType,
						},
					},
				},
			},
			expected: []*model.Transaction{txs[1], txs[4]},
		},
		{
			name: "filter GnoEvent by event type (bad)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						GnoEvent: &model.GnoEventInput{
							Type: &badString,
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter GnoEvent by PkgPath",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						GnoEvent: &model.GnoEventInput{
							PkgPath: &gnoEventPkgPath,
						},
					},
				},
			},
			expected: []*model.Transaction{txs[1], txs[4]},
		},
		{
			name: "filter GnoEvent by PkgPath (bad)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						GnoEvent: &model.GnoEventInput{
							PkgPath: &storageDepositePkgPath,
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter GnoEvent by Attributes",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						GnoEvent: &model.GnoEventInput{
							Attrs: []*model.EventAttributeInput{
								{Key: &gnoEventAttributeKey, Value: &gnoEventAttributeValue},
							},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[1], txs[4]},
		},
		{
			name: "filter GnoEvent by Attributes (bad key)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						GnoEvent: &model.GnoEventInput{
							Attrs: []*model.EventAttributeInput{
								{Key: &badString, Value: &gnoEventAttributeValue},
							},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter GnoEvent by Attributes (bad value)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						GnoEvent: &model.GnoEventInput{
							Attrs: []*model.EventAttributeInput{
								{Key: &gnoEventAttributeKey, Value: &badString},
							},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageDepositEvent by type",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageDepositEvent: &model.StorageDepositEventInput{
							Type: &storageDepositeType,
						},
					},
				},
			},
			expected: []*model.Transaction{txs[2], txs[4]},
		},
		{
			name: "filter StorageDepositEvent by type (bad)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageDepositEvent: &model.StorageDepositEventInput{
							Type: &badString,
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageDepositEvent by bytes delta",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageDepositEvent: &model.StorageDepositEventInput{
							BytesDelta: &storageDepositeBytesDelta,
						},
					},
				},
			},
			expected: []*model.Transaction{txs[2], txs[4]},
		},
		{
			name: "filter StorageDepositEvent by bytes delta (bad)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageDepositEvent: &model.StorageDepositEventInput{
							BytesDelta: &badInt,
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageDepositEvent by fee delta",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageDepositEvent: &model.StorageDepositEventInput{
							FeeDelta: &model.CoinInput{
								Denom:  &ugno,
								Amount: &storageDepositeFeeDeltaAmount,
							},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[2], txs[4]},
		},
		{
			name: "filter StorageDepositEvent by fee delta (bad coin denom)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageDepositEvent: &model.StorageDepositEventInput{
							FeeDelta: &model.CoinInput{
								Denom:  &badString,
								Amount: &storageDepositeFeeDeltaAmount,
							},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageDepositEvent by fee delta (bad coin value)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageDepositEvent: &model.StorageDepositEventInput{
							FeeDelta: &model.CoinInput{
								Denom:  &ugno,
								Amount: &badInt,
							},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageDepositEvent by PkgPath",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageDepositEvent: &model.StorageDepositEventInput{
							PkgPath: &storageDepositePkgPath,
						},
					},
				},
			},
			expected: []*model.Transaction{txs[2], txs[4]},
		},
		{
			name: "filter StorageDepositEvent by PkgPath (bad)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageDepositEvent: &model.StorageDepositEventInput{
							PkgPath: &badString,
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageUnlockEvent by type",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageUnlockEvent: &model.StorageUnlockEventInput{
							Type: &storageUnlockType,
						},
					},
				},
			},
			expected: []*model.Transaction{txs[3], txs[4]},
		},
		{
			name: "filter StorageUnlockEvent by type (bad)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageUnlockEvent: &model.StorageUnlockEventInput{
							Type: &badString,
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageUnlockEvent by bytes delta",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageUnlockEvent: &model.StorageUnlockEventInput{
							BytesDelta: &storageUnlockBytesDelta,
						},
					},
				},
			},
			expected: []*model.Transaction{txs[3], txs[4]},
		},
		{
			name: "filter StorageUnlockEvent by bytes delta (bad)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageUnlockEvent: &model.StorageUnlockEventInput{
							BytesDelta: &badInt,
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageUnlockEvent by fee delta",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageUnlockEvent: &model.StorageUnlockEventInput{
							FeeRefund: &model.CoinInput{
								Denom:  &ugno,
								Amount: &storageUnlockFeeRefundAmount,
							},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[3], txs[4]},
		},
		{
			name: "filter StorageUnlockEvent by fee delta (bad coin denom)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageUnlockEvent: &model.StorageUnlockEventInput{
							FeeRefund: &model.CoinInput{
								Denom:  &badString,
								Amount: &storageUnlockFeeRefundAmount,
							},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageUnlockEvent by fee delta (bad coin value)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageUnlockEvent: &model.StorageUnlockEventInput{
							FeeRefund: &model.CoinInput{
								Denom:  &ugno,
								Amount: &badInt,
							},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageUnlockEvent by PkgPath",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageUnlockEvent: &model.StorageUnlockEventInput{
							PkgPath: &storageUnlockPkgPath,
						},
					},
				},
			},
			expected: []*model.Transaction{txs[3], txs[4]},
		},
		{
			name: "filter StorageUnlockEvent by PkgPath (bad)",
			filter: model.TransactionFilter{
				Events: []*model.EventInput{
					{
						StorageUnlockEvent: &model.StorageUnlockEventInput{
							PkgPath: &badString,
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var succeeded []*model.Transaction

			for _, tx := range txs {
				if FilteredTransactionBy(tx, tt.filter) {
					succeeded = append(succeeded, tx)
				}
			}

			require.Equal(t, len(tt.expected), len(succeeded), "The number of filtered transactions should match the expected number")
			for i, txExpected := range tt.expected {
				assert.Equal(
					t, txExpected, succeeded[i],
					fmt.Sprintf(
						"The filtered transaction should match the expected transaction: %v",
						tt.expected[i],
					),
				)
			}
		})
	}
}
