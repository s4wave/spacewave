package common

import (
	"bytes"
	"context"
	"database/sql"
	"strings"

	"github.com/s4wave/spacewave/db/kvtx"
)

// Iterator iterates over SQLite results using direct SELECTs on the queryer.
//
// This intentionally does not hold an explicit read transaction open. Each
// Seek/Next is a standalone query, which removes BEGIN/ROLLBACK overhead at the
// cost of allowing iteration to observe concurrent writes.
type Iterator struct {
	ctx     context.Context
	queryer sqliteQueryer
	active  func() error
	table   string
	prefix  []byte
	reverse bool

	// Current position
	currentKey   []byte
	currentValue []byte

	// State
	err     error
	valid   bool
	started bool
	closed  bool

	// Precomputed queries for advance operation
	advanceForwardQuery        string
	advanceForwardPrefixQuery  string
	advanceBackwardQuery       string
	advanceBackwardPrefixQuery string

	// Precomputed queries for seek operation
	seekForwardQuery           string
	seekForwardPrefixQuery     string
	seekForwardPrefixNilQuery  string
	seekBackwardQuery          string
	seekBackwardPrefixQuery    string
	seekBackwardPrefixNilQuery string
	seekAbsoluteStartQuery     string
	seekAbsoluteEndQuery       string
}

// NewIterator constructs a new SQLite iterator.
func NewIterator(ctx context.Context, queryer sqliteQueryer, active func() error, table string, prefix []byte, sort, reverse bool) *Iterator {
	i := &Iterator{
		ctx:     ctx,
		queryer: queryer,
		active:  active,
		table:   table,
		prefix:  prefix,
		reverse: reverse,
	}

	i.advanceForwardQuery = strings.Join([]string{"SELECT key, value FROM", table, "WHERE key > ? ORDER BY key LIMIT 1"}, " ")
	i.advanceForwardPrefixQuery = strings.Join([]string{"SELECT key, value FROM", table, "WHERE key > ? AND key >= ? AND key < ? ORDER BY key LIMIT 1"}, " ")
	i.advanceBackwardQuery = strings.Join([]string{"SELECT key, value FROM", table, "WHERE key < ? ORDER BY key DESC LIMIT 1"}, " ")
	i.advanceBackwardPrefixQuery = strings.Join([]string{"SELECT key, value FROM", table, "WHERE key >= ? AND key < ? AND key < ? ORDER BY key DESC LIMIT 1"}, " ")

	i.seekForwardQuery = strings.Join([]string{"SELECT key, value FROM", table, "WHERE key >= ? ORDER BY key LIMIT 1"}, " ")
	i.seekForwardPrefixQuery = strings.Join([]string{"SELECT key, value FROM", table, "WHERE key >= ? AND key < ? ORDER BY key LIMIT 1"}, " ")
	i.seekForwardPrefixNilQuery = strings.Join([]string{"SELECT key, value FROM", table, "WHERE key >= ? AND key < ? ORDER BY key LIMIT 1"}, " ")
	i.seekBackwardQuery = strings.Join([]string{"SELECT key, value FROM", table, "WHERE key <= ? ORDER BY key DESC LIMIT 1"}, " ")
	i.seekBackwardPrefixQuery = strings.Join([]string{"SELECT key, value FROM", table, "WHERE key >= ? AND key <= ? AND key < ? ORDER BY key DESC LIMIT 1"}, " ")
	i.seekBackwardPrefixNilQuery = strings.Join([]string{"SELECT key, value FROM", table, "WHERE key >= ? AND key < ? ORDER BY key DESC LIMIT 1"}, " ")
	i.seekAbsoluteStartQuery = strings.Join([]string{"SELECT key, value FROM", table, "ORDER BY key LIMIT 1"}, " ")
	i.seekAbsoluteEndQuery = strings.Join([]string{"SELECT key, value FROM", table, "ORDER BY key DESC LIMIT 1"}, " ")

	return i
}

// Err returns any error that has closed the iterator.
func (i *Iterator) Err() error {
	return i.err
}

// Valid returns if the iterator points to a valid entry.
func (i *Iterator) Valid() bool {
	return !i.closed && i.valid && i.err == nil
}

// Key returns the current entry key, or nil if not valid.
func (i *Iterator) Key() []byte {
	if !i.Valid() {
		return nil
	}
	return i.currentKey
}

// Value returns the current entry value, or nil if not valid.
func (i *Iterator) Value() ([]byte, error) {
	if !i.Valid() {
		return nil, i.Err()
	}
	return i.currentValue, nil
}

// ValueCopy copies the value to the given byte slice and returns it.
func (i *Iterator) ValueCopy(bt []byte) ([]byte, error) {
	if err := i.Err(); err != nil {
		return nil, err
	}
	if !i.Valid() {
		return nil, nil
	}
	return append(bt[:0], i.currentValue...), nil
}

// Next advances to the next entry and returns Valid.
func (i *Iterator) Next() bool {
	if i.active != nil {
		if err := i.active(); err != nil {
			i.err = err
			return false
		}
	}
	if i.closed || i.err != nil {
		return false
	}

	if !i.started {
		if err := i.Seek(nil); err != nil {
			return false
		}
		return i.Valid()
	}

	return i.advance()
}

// advance moves to the next key in sequence.
func (i *Iterator) advance() bool {
	if i.currentKey == nil {
		return false
	}

	var query string
	var args []any

	if i.reverse {
		if len(i.prefix) > 0 {
			upperBound := CreateUpperBound(i.prefix)
			query = i.advanceBackwardPrefixQuery
			args = []any{i.prefix, i.currentKey, upperBound}
		} else {
			query = i.advanceBackwardQuery
			args = []any{i.currentKey}
		}
	} else {
		if len(i.prefix) > 0 {
			upperBound := CreateUpperBound(i.prefix)
			query = i.advanceForwardPrefixQuery
			args = []any{i.currentKey, i.prefix, upperBound}
		} else {
			query = i.advanceForwardQuery
			args = []any{i.currentKey}
		}
	}

	var key, value []byte
	err := i.queryer.QueryRowContext(i.ctx, query, args...).Scan(&key, &value)
	if err == sql.ErrNoRows {
		i.valid = false
		return false
	}
	if err != nil {
		i.err = err
		return false
	}

	if len(i.prefix) > 0 && !bytes.HasPrefix(key, i.prefix) {
		i.valid = false
		return false
	}

	i.currentKey = key
	i.currentValue = value
	i.valid = true
	return true
}

// Seek moves the iterator to the selected key, or the next key after the key.
func (i *Iterator) Seek(k []byte) error {
	if i.active != nil {
		if err := i.active(); err != nil {
			i.err = err
			return err
		}
	}
	if i.closed {
		return context.Canceled
	}

	i.started = true

	var query string
	var args []any

	if i.reverse {
		if len(i.prefix) > 0 {
			upperBound := CreateUpperBound(i.prefix)
			if k == nil {
				query = i.seekBackwardPrefixNilQuery
				args = []any{i.prefix, upperBound}
			} else {
				query = i.seekBackwardPrefixQuery
				args = []any{i.prefix, k, upperBound}
			}
		} else {
			if k == nil {
				query = i.seekAbsoluteEndQuery
			} else {
				query = i.seekBackwardQuery
				args = []any{k}
			}
		}
	} else {
		if len(i.prefix) > 0 {
			upperBound := CreateUpperBound(i.prefix)
			if k == nil {
				query = i.seekForwardPrefixNilQuery
				args = []any{i.prefix, upperBound}
			} else {
				seekKey := k
				if bytes.Compare(k, i.prefix) < 0 {
					seekKey = i.prefix
				}
				query = i.seekForwardPrefixQuery
				args = []any{seekKey, upperBound}
			}
		} else {
			if k == nil {
				query = i.seekAbsoluteStartQuery
			} else {
				query = i.seekForwardQuery
				args = []any{k}
			}
		}
	}

	var key, value []byte
	err := i.queryer.QueryRowContext(i.ctx, query, args...).Scan(&key, &value)
	if err == sql.ErrNoRows {
		i.valid = false
		return nil
	}
	if err != nil {
		i.err = err
		return err
	}

	if len(i.prefix) > 0 && !bytes.HasPrefix(key, i.prefix) {
		i.valid = false
		return nil
	}

	i.currentKey = key
	i.currentValue = value
	i.valid = true
	return nil
}

// Close closes the iterator.
func (i *Iterator) Close() {
	i.closed = true
	i.valid = false
	i.currentKey = nil
	i.currentValue = nil
	if i.err == nil {
		i.err = context.Canceled
	}
}

// _ is a type assertion
var _ kvtx.Iterator = ((*Iterator)(nil))
