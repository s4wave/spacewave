package common

import (
	"context"
	"database/sql"
	"sync"
)

// urgentReadConns is the number of connections kept for urgent reads, one for
// each block load a World write's batched page loads run at once.
const urgentReadConns = 16

// connKeep holds a fixed number of connections taken from a pool and kept
// out of it. Its user takes one, uses it alone, and puts it back.
//
// A pool keeps a bounded number of idle connections. When a burst of readers,
// such as every client reconnecting after a restart, holds more than that, each
// connection past the bound closes when it returns, and the next caller opens a
// new one: a SQLite open, its pragmas and a cold page cache. A caller every
// write waits for, such as the store's writer or a read under the World's
// writer lock, must not pay that open inside the burst, so it takes its
// connection from a keep, which the pool's churn never closes.
//
// The keep is a channel of n slots. A slot holds a kept connection, or nil
// until its first use opens one. A caller that finds every slot in use takes
// a connection from the pool as before. A connection that failed is discarded
// and its slot opens a new one at its next use.
type connKeep struct {
	// db is the pool connections are taken from.
	db *sql.DB
	// slots holds the free slots.
	slots chan *sql.Conn
	// mtx guards closed.
	mtx sync.Mutex
	// closed closes connections put back after close.
	closed bool
}

// newConnKeep returns a keep of n connections taken from db as needed.
func newConnKeep(db *sql.DB, n int) *connKeep {
	k := &connKeep{db: db, slots: make(chan *sql.Conn, n)}
	for range n {
		k.slots <- nil
	}
	return k
}

// take returns a free slot's connection, taking one from the pool on the
// slot's first use, and false when every slot is in use. It never waits for a
// slot: a reader holding one may read in a transaction of its own, such as in
// a scan's callback, and must not wait on itself. The caller passes the
// connection to put, or to discard when it failed.
func (k *connKeep) take(ctx context.Context) (*sql.Conn, bool, error) {
	// Take a free slot without waiting.
	var conn *sql.Conn
	select {
	case conn = <-k.slots:
	default:
		return nil, false, nil
	}
	if conn != nil {
		return conn, true, nil
	}

	// Open the slot's connection on its first use.
	conn, err := k.db.Conn(ctx)
	if err != nil {
		k.slots <- nil
		return nil, false, err
	}
	return conn, true, nil
}

// put keeps conn, which is outside any transaction, for the next take.
func (k *connKeep) put(conn *sql.Conn) {
	// Close the connection instead once the keep closed.
	k.mtx.Lock()
	closed := k.closed
	k.mtx.Unlock()
	if closed {
		_ = conn.Close()
		conn = nil
	}
	k.slots <- conn
}

// discard drops conn, which may still be inside a transaction, and frees its
// slot.
func (k *connKeep) discard(conn *sql.Conn) {
	discardConn(conn)
	k.slots <- nil
}

// close returns the free slots' connections to the pool, and the others as
// they come back, before the pool closes. A nil keep closes nothing.
func (k *connKeep) close() {
	// A store without keeps has nothing to close.
	if k == nil {
		return
	}

	// Mark the keep closed, then close the free slots' connections.
	k.mtx.Lock()
	k.closed = true
	k.mtx.Unlock()
	free := 0
	for drained := false; !drained; {
		select {
		case conn := <-k.slots:
			if conn != nil {
				_ = conn.Close()
			}
			free++
		default:
			drained = true
		}
	}

	// Free the drained slots again.
	for range free {
		k.slots <- nil
	}
}
