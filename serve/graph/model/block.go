package model

import (
	"strconv"
	"time"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
)

type Block struct {
	b *types.Block
}

func NewBlock(b *types.Block) *Block {
	return &Block{
		b: b,
	}
}

func (b *Block) ID() string {
	return strconv.Itoa(int(b.b.Height))
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

func (b *Block) ProposerAddressRaw() string {
	return b.b.ProposerAddress.String()
}
