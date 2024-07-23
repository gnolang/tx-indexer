package model

import (
	"encoding/base64"
	"strconv"
	"sync"
	"time"

	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
)

type Block struct {
	b   *types.Block
	txs []*BlockTransaction

	onceTxs sync.Once
}

func NewBlock(b *types.Block) *Block {
	return &Block{
		b: b,
	}
}

func (b *Block) ID() string {
	return strconv.Itoa(int(b.b.Height))
}

func (b *Block) Hash() string {
	return base64.StdEncoding.EncodeToString(b.b.Hash())
}

func (b *Block) Height() int64 {
	return b.b.Height
}

func (b *Block) Version() string {
	return b.b.Version
}

func (b *Block) ChainID() string {
	return b.b.ChainID
}

func (b *Block) Time() time.Time {
	return b.b.Time
}

func (b *Block) NumTxs() int64 {
	return b.b.NumTxs
}

func (b *Block) TotalTxs() int64 {
	return b.b.TotalTxs
}

func (b *Block) AppVersion() string {
	return b.b.AppVersion
}

func (b *Block) LastBlockHash() string {
	return base64.StdEncoding.EncodeToString(b.b.LastBlockID.Hash)
}

func (b *Block) ProposerAddressRaw() string {
	return b.b.ProposerAddress.String()
}

func (b *Block) LastCommitHash() string {
	if b.b.LastCommitHash == nil {
		return ""
	}

	return base64.StdEncoding.EncodeToString(b.b.LastCommitHash)
}

func (b *Block) ValidatorsHash() string {
	return base64.StdEncoding.EncodeToString(b.b.ValidatorsHash)
}

func (b *Block) NextValidatorsHash() string {
	return base64.StdEncoding.EncodeToString(b.b.NextValidatorsHash)
}

func (b *Block) ConsensusHash() string {
	return base64.StdEncoding.EncodeToString(b.b.ConsensusHash)
}

func (b *Block) AppHash() string {
	return base64.StdEncoding.EncodeToString(b.b.AppHash)
}

func (b *Block) LastResultsHash() string {
	return base64.StdEncoding.EncodeToString(b.b.LastResultsHash)
}

func (b *Block) Txs() []*BlockTransaction {
	return b.getTxs()
}

func (b *Block) getTxs() []*BlockTransaction {
	// The function to unmarshal a block's transactions is executed once.
	unmarshalTxs := func() {
		var blockTxs []*BlockTransaction

		for _, tx := range b.b.Txs {
			blockTx := NewBlockTransaction(tx)
			if blockTx != nil {
				blockTxs = append(blockTxs, blockTx)
			}
		}

		b.txs = blockTxs
	}

	b.onceTxs.Do(unmarshalTxs)

	return b.txs
}

func NewBlockTransaction(tx types.Tx) *BlockTransaction {
	var stdTx std.Tx
	if err := amino.Unmarshal(tx, &stdTx); err != nil {
		return nil
	}

	return &BlockTransaction{
		Hash:       base64.StdEncoding.EncodeToString(tx.Hash()),
		ContentRaw: tx.String(),
		Fee: &TxFee{
			GasWanted: int(stdTx.Fee.GasWanted),
			GasFee: &Coin{
				Amount: int(stdTx.Fee.GasFee.Amount),
				Denom:  stdTx.Fee.GasFee.Denom,
			},
		},
		Memo: stdTx.Memo,
	}
}
