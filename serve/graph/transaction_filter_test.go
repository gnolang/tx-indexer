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
	gnoEventAttributeKey := "name" //nolint:goconst // need a pointer
	gnoEventAttributeValue := "gno"
	storageDepositeType := "StorageDepositEvent" //nolint:goconst // need a pointer
	storageDepositeBytesDelta := 100
	storageDepositeFeeDeltaAmount := 10
	storageDepositePkgPath := "gno.land/r/gnoland/users/v1"
	storageUnlockType := "StorageUnlockEvent" //nolint:goconst // need a pointer
	storageUnlockBytesDelta := 100
	storageUnlockFeeRefundAmount := 10
	storageUnlockPkgPath := "gno.land/r/gnoland/users/v1"

	tests := []struct {
		name     string
		where    model.FilterTransaction
		expected []*model.Transaction
	}{
		{
			name:     "no filter",
			where:    model.FilterTransaction{},
			expected: txs,
		},
		{
			name: "filter GnoEvent by event type",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						GnoEvent: &model.NestedFilterGnoEvent{
							Type: &model.FilterString{Eq: &gnoEventType},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[1], txs[4]},
		},
		{
			name: "filter GnoEvent by event type (bad)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						GnoEvent: &model.NestedFilterGnoEvent{
							Type: &model.FilterString{Eq: &badString},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter GnoEvent by PkgPath",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						GnoEvent: &model.NestedFilterGnoEvent{
							PkgPath: &model.FilterString{Eq: &gnoEventPkgPath},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[1], txs[4]},
		},
		{
			name: "filter GnoEvent by PkgPath (bad)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						GnoEvent: &model.NestedFilterGnoEvent{
							PkgPath: &model.FilterString{Eq: &storageDepositePkgPath},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter GnoEvent by Attributes",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						GnoEvent: &model.NestedFilterGnoEvent{
							Attrs: &model.NestedFilterGnoEventAttribute{
								Key:   &model.FilterString{Eq: &gnoEventAttributeKey},
								Value: &model.FilterString{Eq: &gnoEventAttributeValue},
							},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[1], txs[4]},
		},
		{
			name: "filter GnoEvent by Attributes (bad key)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						GnoEvent: &model.NestedFilterGnoEvent{
							Attrs: &model.NestedFilterGnoEventAttribute{
								Key:   &model.FilterString{Eq: &badString},
								Value: &model.FilterString{Eq: &gnoEventAttributeValue},
							},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter GnoEvent by Attributes (bad value)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						GnoEvent: &model.NestedFilterGnoEvent{
							Attrs: &model.NestedFilterGnoEventAttribute{
								Key:   &model.FilterString{Eq: &gnoEventAttributeKey},
								Value: &model.FilterString{Eq: &badString},
							},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageDepositEvent by type",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageDepositEvent: &model.NestedFilterStorageDepositEvent{
							Type: &model.FilterString{Eq: &storageDepositeType},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[2], txs[4]},
		},
		{
			name: "filter StorageDepositEvent by type (bad)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageDepositEvent: &model.NestedFilterStorageDepositEvent{
							Type: &model.FilterString{Eq: &badString},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageDepositEvent by bytes delta",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageDepositEvent: &model.NestedFilterStorageDepositEvent{
							BytesDelta: &model.FilterInt{Eq: &storageDepositeBytesDelta},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[2], txs[4]},
		},
		{
			name: "filter StorageDepositEvent by bytes delta (bad)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageDepositEvent: &model.NestedFilterStorageDepositEvent{
							BytesDelta: &model.FilterInt{Eq: &badInt},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageDepositEvent by fee delta",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageDepositEvent: &model.NestedFilterStorageDepositEvent{
							FeeDelta: &model.NestedFilterCoin{
								Denom:  &model.FilterString{Eq: &ugno},
								Amount: &model.FilterInt{Eq: &storageDepositeFeeDeltaAmount},
							},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[2], txs[4]},
		},
		{
			name: "filter StorageDepositEvent by fee delta (bad coin denom)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageDepositEvent: &model.NestedFilterStorageDepositEvent{
							FeeDelta: &model.NestedFilterCoin{
								Denom:  &model.FilterString{Eq: &badString},
								Amount: &model.FilterInt{Eq: &storageDepositeFeeDeltaAmount},
							},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageDepositEvent by fee delta (bad coin value)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageDepositEvent: &model.NestedFilterStorageDepositEvent{
							FeeDelta: &model.NestedFilterCoin{
								Denom:  &model.FilterString{Eq: &ugno},
								Amount: &model.FilterInt{Eq: &badInt},
							},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageDepositEvent by PkgPath",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageDepositEvent: &model.NestedFilterStorageDepositEvent{
							PkgPath: &model.FilterString{Eq: &storageDepositePkgPath},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[2], txs[4]},
		},
		{
			name: "filter StorageDepositEvent by PkgPath (bad)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageDepositEvent: &model.NestedFilterStorageDepositEvent{
							PkgPath: &model.FilterString{Eq: &badString},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageUnlockEvent by type",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageUnlockEvent: &model.NestedFilterStorageUnlockEvent{
							Type: &model.FilterString{Eq: &storageUnlockType},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[3], txs[4]},
		},
		{
			name: "filter StorageUnlockEvent by type (bad)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageUnlockEvent: &model.NestedFilterStorageUnlockEvent{
							Type: &model.FilterString{Eq: &badString},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageUnlockEvent by bytes delta",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageUnlockEvent: &model.NestedFilterStorageUnlockEvent{
							BytesDelta: &model.FilterInt{Eq: &storageUnlockBytesDelta},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[3], txs[4]},
		},
		{
			name: "filter StorageUnlockEvent by bytes delta (bad)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageUnlockEvent: &model.NestedFilterStorageUnlockEvent{
							BytesDelta: &model.FilterInt{Eq: &badInt},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageUnlockEvent by fee refund",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageUnlockEvent: &model.NestedFilterStorageUnlockEvent{
							FeeRefund: &model.NestedFilterCoin{
								Denom:  &model.FilterString{Eq: &ugno},
								Amount: &model.FilterInt{Eq: &storageUnlockFeeRefundAmount},
							},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[3], txs[4]},
		},
		{
			name: "filter StorageUnlockEvent by fee refund (bad coin denom)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageUnlockEvent: &model.NestedFilterStorageUnlockEvent{
							FeeRefund: &model.NestedFilterCoin{
								Denom:  &model.FilterString{Eq: &badString},
								Amount: &model.FilterInt{Eq: &storageUnlockFeeRefundAmount},
							},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageUnlockEvent by fee refund (bad coin value)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageUnlockEvent: &model.NestedFilterStorageUnlockEvent{
							FeeRefund: &model.NestedFilterCoin{
								Denom:  &model.FilterString{Eq: &ugno},
								Amount: &model.FilterInt{Eq: &badInt},
							},
						},
					},
				},
			},
			expected: []*model.Transaction{},
		},
		{
			name: "filter StorageUnlockEvent by PkgPath",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageUnlockEvent: &model.NestedFilterStorageUnlockEvent{
							PkgPath: &model.FilterString{Eq: &storageUnlockPkgPath},
						},
					},
				},
			},
			expected: []*model.Transaction{txs[3], txs[4]},
		},
		{
			name: "filter StorageUnlockEvent by PkgPath (bad)",
			where: model.FilterTransaction{
				Response: &model.NestedFilterTransactionResponse{
					Events: &model.NestedFilterEvent{
						StorageUnlockEvent: &model.NestedFilterStorageUnlockEvent{
							PkgPath: &model.FilterString{Eq: &badString},
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
				if tt.where.Eval(tx) {
					succeeded = append(succeeded, tx)
				}
			}

			require.Equal(t, len(tt.expected), len(succeeded),
				"The number of filtered transactions should match the expected number")

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
