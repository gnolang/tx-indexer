package genesis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gnolang/gno/gno.land/pkg/gnoland"
	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	core_types "github.com/gnolang/gno/tm2/pkg/bft/rpc/core/types"
	"github.com/gnolang/gno/tm2/pkg/bft/state"
	bft_types "github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/crypto"
	p2pTypes "github.com/gnolang/gno/tm2/pkg/p2p/types"
	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/stretchr/testify/require"

	"github.com/gnolang/tx-indexer/internal/mock"
	"github.com/gnolang/tx-indexer/storage"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
)

const testChainID = "test-chain"

// statusFor builds the minimal chain status the bootstrap reads.
func statusFor(chainID string) *core_types.ResultStatus {
	return &core_types.ResultStatus{
		NodeInfo: p2pTypes.NodeInfo{
			Network: chainID,
		},
	}
}

func genesisFor(chainID string, balances []gnoland.Balance, txs []gnoland.TxWithMetadata) *core_types.ResultGenesis {
	return &core_types.ResultGenesis{
		Genesis: &bft_types.GenesisDoc{
			ChainID: chainID,
			AppState: gnoland.GnoGenesisState{
				Balances: balances,
				Txs:      txs,
			},
		},
	}
}

func testBalance(t *testing.T) gnoland.Balance {
	t.Helper()

	return gnoland.Balance{
		Address: crypto.MustAddressFromString("g1u7y667z64x2h7vc6fmpcprgey4ck233jaww9zq"),
		Amount:  std.MustParseCoins("1000ugnot"),
	}
}

// recorder captures what a bootstrap wrote, in order.
type recorder struct {
	batch        *mock.WriteBatch
	latestHeight *uint64
	chainID      string
	writes       []string
	blocks       []*bft_types.Block
	txs          []*bft_types.TxResult
	balances     []gnoland.Balance
}

func newRecorder() *recorder {
	r := &recorder{}

	r.batch = &mock.WriteBatch{
		SetBlockFn: func(block *bft_types.Block) error {
			r.blocks = append(r.blocks, block)
			r.writes = append(r.writes, "block")

			return nil
		},
		SetTxFn: func(tx *bft_types.TxResult) error {
			r.txs = append(r.txs, tx)

			return nil
		},
		SetLatestHeightFn: func(h uint64) error {
			r.latestHeight = &h

			return nil
		},
		SetGenesisBalancesFn: func(balances []gnoland.Balance) error {
			r.balances = balances
			r.writes = append(r.writes, "balances")

			return nil
		},
		SetGenesisChainIDFn: func(chainID string) error {
			r.chainID = chainID
			r.writes = append(r.writes, "chainid")

			return nil
		},
	}

	return r
}

func (r *recorder) storage() *mock.Storage {
	return &mock.Storage{
		GetLatestSavedHeightFn: func() (uint64, error) {
			return 0, storageErrors.ErrNotFound
		},
		GetWriteBatchFn: func() storage.Batch {
			return r.batch
		},
	}
}

// bootstrapOnce runs one attempt, so a failing case reports an error instead of
// retrying until the test times out.
func bootstrapOnce(t *testing.T, store Storage, client Client) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	return Bootstrap(ctx, store, client, WithBackoff(time.Millisecond))
}

// A cold start indexes the genesis block and stores the balances, with the
// chain ID written last so its presence means the whole genesis landed.
func TestBootstrap_ColdStart(t *testing.T) {
	t.Parallel()

	balance := testBalance(t)
	rec := newRecorder()

	client := &mockClient{
		getStatusFn: func() (*core_types.ResultStatus, error) {
			return statusFor(testChainID), nil
		},
		getGenesisFn: func() (*core_types.ResultGenesis, error) {
			return genesisFor(testChainID, []gnoland.Balance{balance}, nil), nil
		},
		getBlockResultsFn: func(uint64) (*core_types.ResultBlockResults, error) {
			return &core_types.ResultBlockResults{Results: &state.ABCIResponses{}}, nil
		},
	}

	require.NoError(t, bootstrapOnce(t, rec.storage(), client))

	require.Equal(t, []string{"block", "balances", "chainid"}, rec.writes)
	require.Equal(t, testChainID, rec.chainID)
	require.Equal(t, []gnoland.Balance{balance}, rec.balances)
	require.Len(t, rec.blocks, 1)
	require.EqualValues(t, 0, rec.blocks[0].Height)
	require.NotNil(t, rec.latestHeight)
	require.EqualValues(t, 0, *rec.latestHeight)
}

// The whole point of persisting: once the chain ID is there, the genesis
// document is never fetched again, and the balances are not even read.
func TestBootstrap_AlreadyDone(t *testing.T) {
	t.Parallel()

	var genesisCalls int

	store := &mock.Storage{
		GetGenesisChainIDFn: func() (string, error) {
			return testChainID, nil
		},
		GetLatestSavedHeightFn: func() (uint64, error) {
			require.Fail(t, "a completed genesis needs no height probe")

			return 0, nil
		},
		GetGenesisBalancesFn: func() ([]gnoland.Balance, error) {
			require.Fail(t, "the completion check must not decode the balances")

			return nil, nil
		},
		GetWriteBatchFn: func() storage.Batch {
			require.Fail(t, "should not write anything")

			return nil
		},
	}

	client := &mockClient{
		getStatusFn: func() (*core_types.ResultStatus, error) {
			return statusFor(testChainID), nil
		},
		getGenesisFn: func() (*core_types.ResultGenesis, error) {
			genesisCalls++

			return nil, nil
		},
	}

	require.NoError(t, bootstrapOnce(t, store, client))
	require.Zero(t, genesisCalls, "the genesis document must not be fetched again")
}

// A storage populated before the genesis record existed has block 0 but no
// chain ID: the balances are fetched and stored, the block is not re-indexed —
// re-indexing it would reset the latest height and force a full re-sync.
func TestBootstrap_BlockIndexedChainIDMissing(t *testing.T) {
	t.Parallel()

	balance := testBalance(t)
	rec := newRecorder()

	store := rec.storage()
	store.GetLatestSavedHeightFn = func() (uint64, error) {
		return 5_000_000, nil
	}

	client := &mockClient{
		getStatusFn: func() (*core_types.ResultStatus, error) {
			return statusFor(testChainID), nil
		},
		getGenesisFn: func() (*core_types.ResultGenesis, error) {
			return genesisFor(testChainID, []gnoland.Balance{balance}, nil), nil
		},
		getBlockResultsFn: func(uint64) (*core_types.ResultBlockResults, error) {
			require.Fail(t, "should not fetch results for an already indexed genesis")

			return nil, nil
		},
	}

	require.NoError(t, bootstrapOnce(t, store, client))

	require.Equal(t, []string{"balances", "chainid"}, rec.writes)
	require.Nil(t, rec.latestHeight, "the latest height must be left alone")
	require.Empty(t, rec.blocks)
}

// A storage holding one chain's genesis must not be served by a node running
// another, and the mismatch must not be retried.
func TestBootstrap_ChainIDMismatch(t *testing.T) {
	t.Parallel()

	t.Run("stored chain ID disagrees with the node", func(t *testing.T) {
		t.Parallel()

		store := &mock.Storage{
			GetGenesisChainIDFn: func() (string, error) {
				return "test3", nil
			},
			GetWriteBatchFn: func() storage.Batch {
				require.Fail(t, "should not write anything")

				return nil
			},
		}

		client := &mockClient{
			getStatusFn: func() (*core_types.ResultStatus, error) {
				return statusFor("test7"), nil
			},
		}

		err := bootstrapOnce(t, store, client)
		require.ErrorIs(t, err, ErrChainIDMismatch)
		require.NotErrorIs(t, err, context.DeadlineExceeded, "a mismatch must not be retried")
	})

	t.Run("fetched genesis disagrees with the node", func(t *testing.T) {
		t.Parallel()

		store := &mock.Storage{
			GetLatestSavedHeightFn: func() (uint64, error) {
				return 0, storageErrors.ErrNotFound
			},
			GetWriteBatchFn: func() storage.Batch {
				require.Fail(t, "should not write anything")

				return nil
			},
		}

		client := &mockClient{
			getStatusFn: func() (*core_types.ResultStatus, error) {
				return statusFor("test7"), nil
			},
			getGenesisFn: func() (*core_types.ResultGenesis, error) {
				return genesisFor("test3", nil, nil), nil
			},
		}

		err := bootstrapOnce(t, store, client)
		require.ErrorIs(t, err, ErrChainIDMismatch)
		require.NotErrorIs(t, err, context.DeadlineExceeded)
	})
}

// A failed fetch must leave nothing behind, so the next attempt starts over
// rather than treating a half-finished bootstrap as done.
func TestBootstrap_RetriedUntilSuccess(t *testing.T) {
	t.Parallel()

	var genesisCalls int

	rec := newRecorder()

	client := &mockClient{
		getStatusFn: func() (*core_types.ResultStatus, error) {
			return statusFor(testChainID), nil
		},
		getGenesisFn: func() (*core_types.ResultGenesis, error) {
			genesisCalls++

			// Fail twice, then answer.
			if genesisCalls < 3 {
				return nil, errors.New("node unreachable")
			}

			return genesisFor(testChainID, nil, nil), nil
		},
		getBlockResultsFn: func(uint64) (*core_types.ResultBlockResults, error) {
			return &core_types.ResultBlockResults{Results: &state.ABCIResponses{}}, nil
		},
	}

	require.NoError(t, bootstrapOnce(t, rec.storage(), client))

	require.Equal(t, 3, genesisCalls, "a failed fetch must be retried, not recorded as done")
	require.Equal(t, []string{"block", "balances", "chainid"}, rec.writes)
}

// Unlike the fetcher's writeSlot, a genesis block that cannot be stored fails
// the bootstrap rather than being logged and skipped: recording the genesis as
// done without block 0 would leave the gap permanently. A storage failure may
// clear, so it stays retryable — it just never completes.
func TestBootstrap_BlockWriteFailurePreventsCompletion(t *testing.T) {
	t.Parallel()

	store := &mock.Storage{
		GetLatestSavedHeightFn: func() (uint64, error) {
			return 0, storageErrors.ErrNotFound
		},
		GetWriteBatchFn: func() storage.Batch {
			return &mock.WriteBatch{
				SetBlockFn: func(*bft_types.Block) error {
					return errors.New("unencodable block")
				},
				SetGenesisChainIDFn: func(string) error {
					require.Fail(t, "the genesis must not be recorded without block 0")

					return nil
				},
			}
		},
	}

	client := &mockClient{
		getStatusFn: func() (*core_types.ResultStatus, error) {
			return statusFor(testChainID), nil
		},
		getGenesisFn: func() (*core_types.ResultGenesis, error) {
			return genesisFor(testChainID, nil, nil), nil
		},
		getBlockResultsFn: func(uint64) (*core_types.ResultBlockResults, error) {
			return &core_types.ResultBlockResults{Results: &state.ABCIResponses{}}, nil
		},
	}

	// Retried, so the deadline is what surfaces; the guard above is what proves
	// the genesis was never recorded as done.
	require.Error(t, bootstrapOnce(t, store, client))
}

// The node answering with fewer results than the genesis has transactions would
// read past the end of the slice.
func TestBootstrap_ShortResults(t *testing.T) {
	t.Parallel()

	txs := []gnoland.TxWithMetadata{
		{Tx: std.Tx{Memo: "genesis tx 0"}},
		{Tx: std.Tx{Memo: "genesis tx 1"}},
	}

	rec := newRecorder()

	client := &mockClient{
		getStatusFn: func() (*core_types.ResultStatus, error) {
			return statusFor(testChainID), nil
		},
		getGenesisFn: func() (*core_types.ResultGenesis, error) {
			return genesisFor(testChainID, nil, txs), nil
		},
		getBlockResultsFn: func(uint64) (*core_types.ResultBlockResults, error) {
			return &core_types.ResultBlockResults{
				Results: &state.ABCIResponses{
					DeliverTxs: make([]abci.ResponseDeliverTx, 1),
				},
			}, nil
		},
	}

	err := bootstrapOnce(t, rec.storage(), client)
	require.ErrorContains(t, err, "short of the genesis block")
	require.NotContains(t, rec.writes, "chainid")
}

func TestBootstrap_InvalidState(t *testing.T) {
	t.Parallel()

	store := &mock.Storage{
		GetLatestSavedHeightFn: func() (uint64, error) {
			return 0, storageErrors.ErrNotFound
		},
		GetWriteBatchFn: func() storage.Batch {
			require.Fail(t, "should not write anything")

			return nil
		},
	}

	t.Run("nil genesis doc", func(t *testing.T) {
		t.Parallel()

		client := &mockClient{
			getStatusFn: func() (*core_types.ResultStatus, error) {
				return statusFor(testChainID), nil
			},
			getGenesisFn: func() (*core_types.ResultGenesis, error) {
				return &core_types.ResultGenesis{}, nil
			},
		}

		require.ErrorIs(t, bootstrapOnce(t, store, client), ErrInvalidState)
	})

	t.Run("not a gno genesis", func(t *testing.T) {
		t.Parallel()

		client := &mockClient{
			getStatusFn: func() (*core_types.ResultStatus, error) {
				return statusFor(testChainID), nil
			},
			getGenesisFn: func() (*core_types.ResultGenesis, error) {
				return &core_types.ResultGenesis{
					Genesis: &bft_types.GenesisDoc{
						ChainID:  testChainID,
						AppState: 0xdeadbeef,
					},
				}, nil
			},
		}

		require.ErrorIs(t, bootstrapOnce(t, store, client), ErrInvalidState)
	})
}

// Cancellation is the only way out of a persistent chain problem.
func TestBootstrap_CancelledWhileRetrying(t *testing.T) {
	t.Parallel()

	store := &mock.Storage{
		GetLatestSavedHeightFn: func() (uint64, error) {
			return 0, storageErrors.ErrNotFound
		},
	}

	client := &mockClient{
		getStatusFn: func() (*core_types.ResultStatus, error) {
			return nil, errors.New("node unreachable")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	require.ErrorIs(
		t,
		Bootstrap(ctx, store, client, WithBackoff(time.Millisecond)),
		context.DeadlineExceeded,
	)
}
