package subs

import "github.com/gnolang/tx-indexer/serve/conns"

// ConnectionFetcher is the WS connection manager abstraction
type ConnectionFetcher interface {
	// GetWSConnection returns the requested WS connection
	GetWSConnection(string) conns.WSConnection
}
