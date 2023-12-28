package subscription

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/serve/conns"
)

// baseSubscription defines the base
// functionality for all subscription types
type baseSubscription struct {
	conn conns.WSConnection
}

func newBaseSubscription(conn conns.WSConnection) *baseSubscription {
	return &baseSubscription{
		conn: conn,
	}
}

func (b *baseSubscription) WriteResponse(_ *types.Block) error { return nil }
