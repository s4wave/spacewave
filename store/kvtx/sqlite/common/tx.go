package common

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"

	"github.com/aperturerobotics/hydra/kvtx"
)

// Tx is a SQLite transaction.
type Tx struct {
	txn         *sql.Tx
	table       string
	write       bool
	discardOnce sync.Once

	// Precomputed queries
	getQuery        string
	sizeQuery       string
	setQuery        string
	scanAllQuery    string
	scanPrefixQuery string
	deleteQuery     string
	existsQuery     string
}

// NewTx constructs a new SQLite transaction.
func NewTx(txn *sql.Tx, table string, write bool) *Tx {
	return &Tx{
		txn:   txn,
		table: table,
		write: write,

		// Precompute queries
		getQuery:        strings.Join([]string{"SELECT value FROM", table, "WHERE key = ?"}, " "),
		sizeQuery:       strings.Join([]string{"SELECT COUNT(*) FROM", table}, " "),
		setQuery:        strings.Join([]string{"INSERT OR REPLACE INTO", table, "(key, value) VALUES (?, ?)"}, " "),
		scanAllQuery:    strings.Join([]string{"SELECT key, value FROM", table, "ORDER BY key"}, " "),
		scanPrefixQuery: strings.Join([]string{"SELECT key, value FROM", table, "WHERE key >= ? AND key < ? ORDER BY key"}, " "),
		deleteQuery:     strings.Join([]string{"DELETE FROM", table, "WHERE key = ?"}, " "),
		existsQuery:     strings.Join([]string{"SELECT 1 FROM", table, "WHERE key = ? LIMIT 1"}, " "),
	}
}

// Get returns values for a key.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}

	var value []byte
	err := t.txn.QueryRowContext(ctx, t.getQuery, key).Scan(&value)
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
	var count uint64
	err := t.txn.QueryRowContext(ctx, t.sizeQuery).Scan(&count)
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

	_, err := t.txn.ExecContext(ctx, t.setQuery, key, value)
	return err
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	var query string
	var args []any

	if len(prefix) == 0 {
		query = t.scanAllQuery
	} else {
		query = t.scanPrefixQuery
		upperBound := CreateUpperBound(prefix)
		args = []any{prefix, upperBound}
	}

	rows, err := t.txn.QueryContext(ctx, query, args...)
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
	return NewIterator(ctx, t.txn, t.table, prefix, sort, reverse)
}

// Delete deletes a key.
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}

	_, err := t.txn.ExecContext(ctx, t.deleteQuery, key)
	return err
}

// Commit commits the transaction to storage.
func (t *Tx) Commit(ctx context.Context) error {
	var done bool
	var err error
	t.discardOnce.Do(func() {
		err = t.txn.Commit()
		done = true
	})
	if err != nil {
		return err
	}
	if !done {
		return errors.New("commit called after discard")
	}
	return nil
}

// Exists checks if a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}

	var exists int
	err := t.txn.QueryRowContext(ctx, t.existsQuery, key).Scan(&exists)
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
	t.discardOnce.Do(func() {
		_ = t.txn.Rollback()
	})
}

// CreateUpperBound creates an upper bound for prefix scanning by incrementing the last byte.
func CreateUpperBound(prefix []byte) []byte {
	if len(prefix) == 0 {
		return nil
	}

	upperBound := make([]byte, len(prefix))
	copy(upperBound, prefix)

	// Find the rightmost byte that can be incremented
	for i := len(upperBound) - 1; i >= 0; i-- {
		if upperBound[i] < 255 {
			upperBound[i]++
			return upperBound[:i+1]
		}
	}

	// All bytes are 255, return nil to indicate no upper bound
	return nil
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))
