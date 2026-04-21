package common

import (
	"context"
	"database/sql"
	"strings"
	"sync"

	"github.com/s4wave/spacewave/db/kvtx"
)

type sqliteQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type sqliteExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Tx is a SQLite-backed kvtx transaction or read handle.
//
// Writes use a real *sql.Tx. Read-only handles run directly on *sql.DB so
// one-shot reads do not pay explicit BEGIN/ROLLBACK overhead.
type Tx struct {
	txn          *sql.Tx
	queryer      sqliteQueryer
	execer       sqliteExecer
	table        string
	write        bool
	closed       bool
	finalizeMu   sync.RWMutex
	finalizeOnce sync.Once

	// Precomputed queries
	getQuery        string
	sizeQuery       string
	setQuery        string
	scanAllQuery    string
	scanPrefixQuery string
	deleteQuery     string
	existsQuery     string
}

func buildQueries(table string) (getQuery, sizeQuery, setQuery, scanAllQuery, scanPrefixQuery, deleteQuery, existsQuery string) {
	getQuery = strings.Join([]string{"SELECT value FROM", table, "WHERE key = ?"}, " ")
	sizeQuery = strings.Join([]string{"SELECT COUNT(*) FROM", table}, " ")
	setQuery = strings.Join([]string{"INSERT OR REPLACE INTO", table, "(key, value) VALUES (?, ?)"}, " ")
	scanAllQuery = strings.Join([]string{"SELECT key, value FROM", table, "ORDER BY key"}, " ")
	scanPrefixQuery = strings.Join([]string{"SELECT key, value FROM", table, "WHERE key >= ? AND key < ? ORDER BY key"}, " ")
	deleteQuery = strings.Join([]string{"DELETE FROM", table, "WHERE key = ?"}, " ")
	existsQuery = strings.Join([]string{"SELECT 1 FROM", table, "WHERE key = ? LIMIT 1"}, " ")
	return
}

// NewTx constructs a new SQLite write transaction.
func NewTx(txn *sql.Tx, table string, write bool) *Tx {
	getQuery, sizeQuery, setQuery, scanAllQuery, scanPrefixQuery, deleteQuery, existsQuery := buildQueries(table)
	return &Tx{
		txn:             txn,
		queryer:         txn,
		execer:          txn,
		table:           table,
		write:           write,
		getQuery:        getQuery,
		sizeQuery:       sizeQuery,
		setQuery:        setQuery,
		scanAllQuery:    scanAllQuery,
		scanPrefixQuery: scanPrefixQuery,
		deleteQuery:     deleteQuery,
		existsQuery:     existsQuery,
	}
}

// NewReadTx constructs a lightweight SQLite read handle backed directly by sql.DB.
func NewReadTx(db *sql.DB, table string) *Tx {
	getQuery, sizeQuery, setQuery, scanAllQuery, scanPrefixQuery, deleteQuery, existsQuery := buildQueries(table)
	return &Tx{
		queryer:         db,
		table:           table,
		getQuery:        getQuery,
		sizeQuery:       sizeQuery,
		setQuery:        setQuery,
		scanAllQuery:    scanAllQuery,
		scanPrefixQuery: scanPrefixQuery,
		deleteQuery:     deleteQuery,
		existsQuery:     existsQuery,
	}
}

func (t *Tx) errIfClosed() error {
	t.finalizeMu.RLock()
	defer t.finalizeMu.RUnlock()
	if t.closed {
		return kvtx.ErrDiscarded
	}
	return nil
}

func (t *Tx) markClosed() {
	t.finalizeMu.Lock()
	t.closed = true
	t.finalizeMu.Unlock()
}

// Get returns values for a key.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	if err := t.errIfClosed(); err != nil {
		return nil, false, err
	}

	var value []byte
	err := t.queryer.QueryRowContext(ctx, t.getQuery, key).Scan(&value)
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
	if err := t.errIfClosed(); err != nil {
		return 0, err
	}

	var count uint64
	err := t.queryer.QueryRowContext(ctx, t.sizeQuery).Scan(&count)
	return count, err
}

// Set sets the value of a key.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if err := t.errIfClosed(); err != nil {
		return err
	}

	_, err := t.execer.ExecContext(ctx, t.setQuery, key, value)
	return err
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	if err := t.errIfClosed(); err != nil {
		return err
	}

	var query string
	var args []any

	if len(prefix) == 0 {
		query = t.scanAllQuery
	} else {
		query = t.scanPrefixQuery
		upperBound := CreateUpperBound(prefix)
		args = []any{prefix, upperBound}
	}

	rows, err := t.queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

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
	if err := t.errIfClosed(); err != nil {
		return kvtx.NewErrIterator(err)
	}
	return NewIterator(ctx, t.queryer, t.errIfClosed, t.table, prefix, sort, reverse)
}

// Delete deletes a key.
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if err := t.errIfClosed(); err != nil {
		return err
	}

	_, err := t.execer.ExecContext(ctx, t.deleteQuery, key)
	return err
}

// Commit commits the transaction to storage.
func (t *Tx) Commit(ctx context.Context) error {
	if !t.write {
		if err := t.errIfClosed(); err != nil {
			return err
		}
		t.finalizeOnce.Do(t.markClosed)
		return nil
	}

	var (
		err       error
		committed bool
	)
	t.finalizeOnce.Do(func() {
		t.markClosed()
		err = t.txn.Commit()
		committed = true
	})
	if !committed {
		return kvtx.ErrDiscarded
	}
	if err != nil {
		return err
	}
	return nil
}

// Exists checks if a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	if err := t.errIfClosed(); err != nil {
		return false, err
	}

	var exists int
	err := t.queryer.QueryRowContext(ctx, t.existsQuery, key).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Discard cancels the transaction.
func (t *Tx) Discard() {
	t.finalizeOnce.Do(func() {
		t.markClosed()
		if t.txn != nil {
			_ = t.txn.Rollback()
		}
	})
}

// CreateUpperBound creates an upper bound for prefix scanning by incrementing the last byte.
func CreateUpperBound(prefix []byte) []byte {
	if len(prefix) == 0 {
		return nil
	}

	upperBound := make([]byte, len(prefix))
	copy(upperBound, prefix)

	// Find the rightmost byte that can be incremented.
	for i := len(upperBound) - 1; i >= 0; i-- {
		if upperBound[i] < 0xff {
			upperBound[i]++
			return upperBound[:i+1]
		}
	}

	// All bytes are 255, return nil to indicate no upper bound
	return nil
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
