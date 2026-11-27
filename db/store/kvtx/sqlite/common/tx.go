package common

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
)

// sqliteQueryer runs reading statements.
type sqliteQueryer interface {
	// QueryRowContext runs a statement that returns at most one row.
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	// QueryContext runs a statement that returns rows.
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Tx is a SQLite-backed kvtx transaction.
//
// A write transaction runs on a connection of its own between BEGIN IMMEDIATE
// and its COMMIT or ROLLBACK, during the store's writer turn. A read
// transaction with snapshot reads opens a deferred transaction on a
// connection of its own at its first read, so every read sees one snapshot.
// Without snapshot reads, it reads through the pool, each statement in its own
// snapshot.
type Tx struct {
	// queries are the store's statements.
	queries *queries
	// db is the pool a read transaction reads through or takes its connection
	// from.
	db *sql.DB
	// conn is the transaction's connection, nil until a snapshot read begins
	// and for reads through the pool.
	conn *sql.Conn
	// snapshot runs a read transaction on a connection of its own.
	snapshot bool
	// write marks a write transaction.
	write bool
	// failure records in flusher a failed WAL flush of the file by another
	// Store, and returns the Store's failure, as Store.failure describes.
	failure func() error
	// flusher counts a write transaction's commit and makes it durable.
	flusher *kvtx.OrderedFlusher
	// flow wakes the store's checkpointer after a commit, and holds back a
	// write transaction whose commit outran it.
	flow *walFlow
	// release ends a write transaction's writer turn.
	release func()
	// turns admits a snapshot read's statements into SQLite, nil for writes,
	// urgent reads and reads through the pool.
	turns readTurns
	// keep is where the transaction's connection comes from and returns to,
	// or nil for the pool.
	keep *connKeep

	// finalizeMu guards closed.
	finalizeMu sync.RWMutex
	// closed rejects operations after Commit or Discard.
	closed bool
	// finalizeOnce lets the first Commit or Discard finish the transaction.
	finalizeOnce atomic.Bool
}

// newWriteTx returns a write transaction during the writer turn that release
// ends. beginWrite opens and begins its connection from keep or db.
func newWriteTx(db *sql.DB, keep *connKeep, queries *queries, failure func() error, flusher *kvtx.OrderedFlusher, flow *walFlow, release func()) *Tx {
	return &Tx{
		queries: queries,
		db:      db,
		keep:    keep,
		write:   true,
		failure: failure,
		flusher: flusher,
		flow:    flow,
		release: release,
	}
}

// newReadTx returns a read transaction on db, in one snapshot when snapshot
// is set, whose statements take turns when turns is set. A snapshot takes its
// connection from keep when it is set, and from db otherwise.
func newReadTx(db *sql.DB, queries *queries, snapshot bool, turns readTurns, keep *connKeep) *Tx {
	return &Tx{queries: queries, db: db, snapshot: snapshot, turns: turns, keep: keep}
}

// beginWrite takes the write transaction's connection and begins its SQLite
// transaction. On failure it drops a connection whose BEGIN failed and ends
// the writer turn.
func (t *Tx) beginWrite(ctx context.Context) error {
	// Take the kept or pooled connection, ending the writer turn on failure.
	conn, err := t.takeConn(ctx)
	if err != nil {
		t.release()
		return err
	}

	// Take SQLite's write lock before returning the transaction to its caller.
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.dropConn(conn)
		t.release()
		return err
	}
	t.conn = conn
	return nil
}

// errIfClosed returns kvtx.ErrDiscarded after Commit or Discard.
func (t *Tx) errIfClosed() error {
	t.finalizeMu.RLock()
	defer t.finalizeMu.RUnlock()
	if t.closed {
		return kvtx.ErrDiscarded
	}
	return nil
}

// markClosed rejects later operations.
func (t *Tx) markClosed() {
	t.finalizeMu.Lock()
	t.closed = true
	t.finalizeMu.Unlock()
}

// queryer returns what the transaction reads through, beginning a snapshot
// read's transaction at its first read.
func (t *Tx) queryer(ctx context.Context) (sqliteQueryer, error) {
	// Refuse reads after Commit or Discard.
	if err := t.errIfClosed(); err != nil {
		return nil, err
	}

	// Read through the transaction's connection, or the pool without a
	// snapshot.
	if t.conn != nil {
		return t.conn, nil
	}
	if !t.snapshot {
		return t.db, nil
	}

	// Begin the snapshot on a connection of its own. The first read fixes
	// the snapshot.
	conn, err := t.takeConn(ctx)
	if err != nil {
		return nil, err
	}
	if err := t.turns.enter(ctx); err != nil {
		t.putConn(conn)
		return nil, err
	}
	_, err = conn.ExecContext(ctx, "BEGIN")
	t.turns.leave()
	if err != nil {
		t.dropConn(conn)
		return nil, err
	}
	t.conn = conn
	return conn, nil
}

// execer returns the write transaction's connection.
func (t *Tx) execer() (*sql.Conn, error) {
	if !t.write {
		return nil, kvtx.ErrNotWrite
	}
	if err := t.errIfClosed(); err != nil {
		return nil, err
	}
	return t.conn, nil
}

// Get returns values for a key.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	// Require a key and an open SQLite transaction before reading its value.
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	q, err := t.queryer(ctx)
	if err != nil {
		return nil, false, err
	}

	// Read the SQLite value and distinguish a missing key from query failure.
	var value []byte
	err = t.turns.queryRow(ctx, q, t.queries.get, []any{key}, &value)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

// Size returns the number of keys in the store.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	// Require an open SQLite transaction before counting its keys.
	q, err := t.queryer(ctx)
	if err != nil {
		return 0, err
	}

	// Count the table's rows.
	var count uint64
	err = t.turns.queryRow(ctx, q, t.queries.size, nil, &count)
	return count, err
}

// Set sets the value of a key.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	// Require a key and an open write transaction before setting a value.
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	conn, err := t.execer()
	if err != nil {
		return err
	}

	// Store the key and value through the transaction's connection.
	_, err = conn.ExecContext(ctx, t.queries.set, key, value)
	return err
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	// Require an open SQLite transaction before scanning its keys.
	q, err := t.queryer(ctx)
	if err != nil {
		return err
	}

	// Select the SQLite scan query and bounds for the requested prefix.
	var query string
	var args []any
	if len(prefix) == 0 {
		query = t.queries.scanAll
	} else if upperBound, ok := kvtx.PrefixSuccessor(prefix); ok {
		query = t.queries.scanPrefix
		args = []any{prefix, upperBound}
	} else {
		query = t.queries.scanPrefixUnbounded
		args = []any{prefix}
	}

	// Open SQLite scan rows and close them when scanning ends, taking a turn
	// for each row but not for the callback, which may read again.
	if err := t.turns.enter(ctx); err != nil {
		return err
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		t.turns.leave()
		return err
	}
	inTurn := true
	defer func() {
		if !inTurn {
			_ = t.turns.enter(context.Background())
		}
		_ = rows.Close()
		t.turns.leave()
	}()

	// Deliver each SQLite key and value to the scan callback.
	for {
		if !rows.Next() {
			return rows.Err()
		}
		var key, value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		t.turns.leave()
		inTurn = false
		if err := cb(key, value); err != nil {
			return err
		}
		if err := t.turns.enter(ctx); err != nil {
			return err
		}
		inTurn = true
	}
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		return cb(key)
	})
}

// Iterate returns an iterator with a given key prefix.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	// Open the transaction's connection, then iterate on it under its turns.
	q, err := t.queryer(ctx)
	if err != nil {
		return kvtx.NewErrIterator(err)
	}
	return NewIterator(ctx, q, t.errIfClosed, t.turns, t.queries.table, prefix, sort, reverse)
}

// Delete deletes a key.
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	// Require a key and an open write transaction before deleting a value.
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	conn, err := t.execer()
	if err != nil {
		return err
	}

	// Remove the key through the transaction's connection.
	_, err = conn.ExecContext(ctx, t.queries.delete, key)
	return err
}

// Exists checks if a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	// Require a key and an open SQLite transaction before checking key existence.
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	q, err := t.queryer(ctx)
	if err != nil {
		return false, err
	}

	// Check SQLite key existence and distinguish absence from query failure.
	var exists int
	err = t.turns.queryRow(ctx, q, t.queries.exists, []any{key}, &exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// existsBatchKeys bounds the keys one ExistsBatch query selects, below
// SQLite's oldest default limit of 999 bound parameters.
const existsBatchKeys = 900

// ExistsBatch reports the presence of keys in caller order, with one query
// per existsBatchKeys keys.
func (t *Tx) ExistsBatch(ctx context.Context, keys [][]byte) ([]bool, error) {
	// Require keys and an open SQLite transaction.
	for _, key := range keys {
		if len(key) == 0 {
			return nil, kvtx.ErrEmptyKey
		}
	}
	q, err := t.queryer(ctx)
	if err != nil {
		return nil, err
	}

	// Query each chunk, then mark the keys it returned.
	present := make(map[string]struct{}, len(keys))
	for chunk := range slices.Chunk(keys, existsBatchKeys) {
		if err := t.existsChunk(ctx, q, chunk, present); err != nil {
			return nil, err
		}
	}
	found := make([]bool, len(keys))
	for i, key := range keys {
		_, found[i] = present[string(key)]
	}
	return found, nil
}

// existsChunk adds to present each of keys the table holds, in one query
// and one read turn.
func (t *Tx) existsChunk(ctx context.Context, q sqliteQueryer, keys [][]byte, present map[string]struct{}) error {
	// Build the query over the chunk's keys.
	args := make([]any, len(keys))
	for i, key := range keys {
		args[i] = key
	}
	query := "SELECT key FROM " + t.queries.table + " WHERE key IN (" + // #nosec G202 -- the table name is built in, and the keys are placeholders.
		strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",") + ")"

	// Read the present keys in one turn.
	if err := t.turns.enter(ctx); err != nil {
		return err
	}
	defer t.turns.leave()
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key []byte
		if err := rows.Scan(&key); err != nil {
			return err
		}
		present[string(key)] = struct{}{}
	}
	return rows.Err()
}

// Commit commits the transaction and returns once it is durable, or commits
// it ordered when ctx carries kvtx.WithOrderedCommitContext. A read
// transaction ends its snapshot. A commit that outran the checkpointer then
// waits for it, as walFlow describes.
func (t *Tx) Commit(ctx context.Context) error {
	// Commit to the WAL, then flush it unless ordering is enough.
	seq, err := t.commit()
	if err != nil || !t.write {
		return err
	}
	if !kvtx.IsOrderedCommitContext(ctx) {
		if err := t.flusher.Sync(ctx); err != nil {
			return err
		}
	}

	// Wait for the checkpointer once the commit has landed.
	if seq != 0 {
		t.flow.wait(ctx, seq)
	}
	return nil
}

// CommitOrdered commits the transaction to the WAL without a flush. A crash
// may lose it only with every commit after it. Sync, a later Commit or the
// store's deadline flush makes it durable. A commit that outran the
// checkpointer then waits for it, as walFlow describes.
func (t *Tx) CommitOrdered(ctx context.Context) error {
	seq, err := t.commit()
	if err == nil && seq != 0 {
		t.flow.wait(ctx, seq)
	}
	return err
}

// commit commits the transaction to the WAL and ends its writer turn, and
// returns the commit's place for walFlow.wait when it outran the
// checkpointer, or zero.
func (t *Tx) commit() (uint64, error) {
	// Let only the first Commit or Discard finish the transaction.
	if !t.finalizeOnce.CompareAndSwap(false, true) {
		return 0, kvtx.ErrDiscarded
	}
	t.markClosed()
	if !t.write {
		t.endRead()
		return 0, nil
	}

	// Commit on the transaction's connection unless a WAL flush failed,
	// rolling back a refused commit so the connection returns to the pool
	// outside a transaction. COMMIT shows the write in the WAL before the
	// flusher syncs it, so Admit orders it against a failing flush: no
	// commit lands once the failure is recorded. A failure of the file in
	// another Store is recorded first, outside Admit.
	_ = t.failure()
	err := t.flusher.Admit(func() error {
		_, err := t.conn.ExecContext(context.Background(), "COMMIT")
		return err
	})
	clean := err == nil
	if !clean {
		_, rerr := t.conn.ExecContext(context.Background(), "ROLLBACK")
		clean = rerr == nil
	}

	// Measure the commit under the turn, end the turn, then count the commit
	// for the flusher and the checkpointer.
	outran := err == nil && t.flow.outran(t.conn)
	t.endWrite(clean)
	if err != nil {
		return 0, err
	}
	t.flusher.Ordered()
	seq := t.flow.committed()
	if !outran {
		return 0, nil
	}
	return seq, nil
}

// Discard cancels the transaction.
func (t *Tx) Discard() {
	// Let only the first Commit or Discard finish the transaction.
	if !t.finalizeOnce.CompareAndSwap(false, true) {
		return
	}
	t.markClosed()
	if !t.write {
		t.endRead()
		return
	}

	// Roll back and end the writer turn.
	_, err := t.conn.ExecContext(context.Background(), "ROLLBACK")
	t.endWrite(err == nil)
}

// endRead ends a snapshot read's transaction and returns its connection.
func (t *Tx) endRead() {
	// A read that never began holds no connection.
	if t.conn == nil {
		return
	}

	// Roll back in a turn, then keep the connection unless the rollback
	// failed.
	_ = t.turns.enter(context.Background())
	_, err := t.conn.ExecContext(context.Background(), "ROLLBACK")
	t.turns.leave()
	if err != nil {
		t.dropConn(t.conn)
		return
	}
	t.putConn(t.conn)
}

// endWrite returns the write transaction's connection to the pool, or
// discards it unless clean, and ends its writer turn.
func (t *Tx) endWrite(clean bool) {
	if clean {
		t.putConn(t.conn)
	} else {
		t.dropConn(t.conn)
	}
	t.release()
}

// takeConn takes the transaction's connection from the keep or the pool.
// A transaction that finds the keep in use reads on a pool connection.
func (t *Tx) takeConn(ctx context.Context) (*sql.Conn, error) {
	if t.keep != nil {
		conn, ok, err := t.keep.take(ctx)
		if ok || err != nil {
			return conn, err
		}
		t.keep = nil
	}
	return t.db.Conn(ctx)
}

// putConn returns conn, outside any transaction, to where it came from.
func (t *Tx) putConn(conn *sql.Conn) {
	if t.keep != nil {
		t.keep.put(conn)
		return
	}
	_ = conn.Close()
}

// dropConn discards conn, which may still be inside a transaction, as
// discardConn does, freeing its place in the keep.
func (t *Tx) dropConn(conn *sql.Conn) {
	if t.keep != nil {
		t.keep.discard(conn)
		return
	}
	discardConn(conn)
}

// discardConn closes conn instead of returning it to the pool. A connection
// whose BEGIN, COMMIT or ROLLBACK failed may still be inside a transaction,
// holding SQLite's write lock, and the pool would hand it out again.
func discardConn(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
}

// _ is a type assertion
var _ kvtx.OrderedCommitTx = (*Tx)(nil)
