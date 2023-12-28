package subs

import "github.com/gnolang/tx-indexer/serve/conns"

type ConnectionFetcher interface {
	GetWSConnection(id string) conns.WSConnection
}
