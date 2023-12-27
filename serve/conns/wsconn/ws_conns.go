package wsconn

import (
	"context"
	"fmt"
	"sync"

	"github.com/gnolang/tx-indexer/serve/conns"
	"github.com/gnolang/tx-indexer/serve/writer"
	"github.com/gnolang/tx-indexer/serve/writer/ws"
	"github.com/olahol/melody"
	"go.uber.org/zap"
)

// Conns manages active WS connections
type Conns struct {
	logger *zap.Logger
	conns  map[string]Conn // ws connection ID -> conn

	mux sync.RWMutex
}

// NewConns creates a new instance of the WS connection manager
func NewConns(logger *zap.Logger) *Conns {
	return &Conns{
		logger: logger,
		conns:  make(map[string]Conn),
	}
}

// AddWSConnection registers a new WS connection
func (pw *Conns) AddWSConnection(id string, session *melody.Session) {
	pw.mux.Lock()
	defer pw.mux.Unlock()

	ctx, cancelFn := context.WithCancel(context.Background())

	pw.conns[id] = Conn{
		ctx:      ctx,
		cancelFn: cancelFn,
		writer: ws.New(
			pw.logger.Named(
				fmt.Sprintf("ws-%s", id),
			),
			session,
		),
	}
}

// RemoveWSConnection removes an existing WS connection
func (pw *Conns) RemoveWSConnection(id string) {
	pw.mux.Lock()
	defer pw.mux.Unlock()

	conn, found := pw.conns[id]
	if !found {
		return
	}

	// Cancel the connection context
	conn.cancelFn()

	delete(pw.conns, id)
}

// GetWSConnection fetches a WS connection, if any
func (pw *Conns) GetWSConnection(id string) conns.WSConnection {
	pw.mux.RLock()
	defer pw.mux.RUnlock()

	conn, found := pw.conns[id]
	if !found {
		return nil
	}

	return &conn
}

// Conn is a single WS connection
type Conn struct {
	ctx      context.Context
	cancelFn context.CancelFunc

	writer writer.ResponseWriter
}

// WriteData writes arbitrary data to the WS connection
func (c *Conn) WriteData(data any) error {
	if c.ctx.Err() != nil {
		return c.ctx.Err()
	}

	c.writer.WriteResponse(data)

	return nil
}
