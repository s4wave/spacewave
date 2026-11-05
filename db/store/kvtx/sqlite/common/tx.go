package common

import (
	"context"
	"database/sql"
	"errors"
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
// connection of its own at its first read, so every read sees one snapshot;
// without them it reads through the pool, each statement in its own snapshot.
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
	// flusher counts a write transaction's commit and makes it durable.
	flusher *kvtx.OrderedFlusher
	// committed wakes the store's checkpointer after a commit.
	committed func()
	// release ends a write transaction's writer turn.
	release func()

	// finalizeMu guards closed.
	finalizeMu sync.RWMutex
	// closed rejects operations after Commit or Discard.
	closed bool
	// finalizeOnce lets the first Commit or Discard finish the transaction.
	finalizeOnce atomic.Bool
}

// newWriteTx returns a write transaction begun on conn during the writer turn
// that release ends.
func newWriteTx(conn *sql.Conn, queries *queries, flusher *kvtx.OrderedFlusher, committed, release func()) *Tx {
	return &Tx{
		queries:   queries,
		conn:      conn,
		write:     true,
		flusher:   flusher,
		committed: committed,
		release:   release,
	}
}

// newReadTx returns a read transaction on db, in one snapshot when snapshot
// is set.
func newReadTx(db *sql.DB, queries *queries, snapshot bool) *Tx {
	return &Tx{queries: queries, db: db, snapshot: snapshot}
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

	// Begin the snapshot on a connection of its own; the first read fixes it.
	conn, err := t.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
		return nil, errors.Join(err, conn.Close())
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
	err = q.QueryRowContext(ctx, t.queries.get, key).Scan(&value)
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
	err = q.QueryRowContext(ctx, t.queries.size).Scan(&count)
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

	// Open SQLite scan rows and release them when scanning ends.
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Deliver each SQLite key and value to the scan callback.
	for rows.Next() {
		var key, value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		if err := cb(key, value); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		return cb(key)
	})
}

// Iterate returns an iterator with a given key prefix.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	q, err := t.queryer(ctx)
	if err != nil {
		return kvtx.NewErrIterator(err)
	}
	return NewIterator(ctx, q, t.errIfClosed, t.queries.table, prefix, sort, reverse)
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
	err = q.QueryRowContext(ctx, t.queries.exists, key).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Commit commits the transaction and returns once it is durable, or commits
// it ordered when ctx carries kvtx.WithOrderedCommitContext. A read
// transaction ends its snapshot.
func (t *Tx) Commit(ctx context.Context) error {
	// Commit to the WAL, then flush it unless ordering is enough.
	if err := t.CommitOrdered(ctx); err != nil || !t.write || kvtx.IsOrderedCommitContext(ctx) {
		return err
	}
	return t.flusher.Sync(ctx)
}

// CommitOrdered commits the transaction to the WAL without a flush. A crash
// may lose it only with every commit after it; Sync, a later Commit or the
// store's deadline flush makes it durable.
func (t *Tx) CommitOrdered(ctx context.Context) error {
	// Let only the first Commit or Discard finish the transaction.
	if !t.finalizeOnce.CompareAndSwap(false, true) {
		return kvtx.ErrDiscarded
	}
	t.markClosed()
	if !t.write {
		t.endRead()
		return nil
	}

	// Commit on the transaction's connection, rolling back a refused commit
	// so the connection returns to the pool outside a transaction.
	_, err := t.conn.ExecContext(context.Background(), "COMMIT")
	if err != nil {
		_, _ = t.conn.ExecContext(context.Background(), "ROLLBACK")
	}

	// End the writer turn, then count the commit for the flusher.
	t.endWrite()
	if err != nil {
		return err
	}
	t.flusher.Ordered()
	t.committed()
	return nil
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
	_, _ = t.conn.ExecContext(context.Background(), "ROLLBACK")
	t.endWrite()
}

// endRead ends a snapshot read's transaction and returns its connection.
func (t *Tx) endRead() {
	if t.conn == nil {
		return
	}
	_, _ = t.conn.ExecContext(context.Background(), "ROLLBACK")
	_ = t.conn.Close()
}

// endWrite returns the write transaction's connection and ends its writer
// turn.
func (t *Tx) endWrite() {
	_ = t.conn.Close()
	t.release()
}

// _ is a type assertion
var _ kvtx.OrderedCommitTx = (*Tx)(nil)
