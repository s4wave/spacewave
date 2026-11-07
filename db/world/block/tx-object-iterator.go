package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// txObjectIterator implements ObjectIterator for a Tx.
type txObjectIterator struct {
	// t is the transaction
	t *Tx
	// ctx is the context
	ctx context.Context
	// it is the state iterator, held open across steps
	it world.ObjectIterator
	// err is any error that occurred
	err error
	// currKey is the current key if valid
	currKey string
	// valid indicates if the iterator is valid
	valid bool
}

// Err returns any error that has closed the iterator.
func (t *txObjectIterator) Err() error {
	return t.err
}

// Valid returns if the iterator points to a valid entry.
func (t *txObjectIterator) Valid() bool {
	return t.err == nil && t.valid
}

// Key returns the current entry key, or empty string if not valid.
func (t *txObjectIterator) Key() string {
	if !t.Valid() {
		return ""
	}
	return t.currKey
}

// Next advances to the next entry and returns Valid.
func (t *txObjectIterator) Next() bool {
	_ = t.step(func() error {
		t.it.Next()
		return nil
	})
	return t.Valid()
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
func (t *txObjectIterator) Seek(k string) error {
	return t.step(func() error {
		return t.it.Seek(k)
	})
}

// step moves the inner iterator with move under the transaction's read lock
// and records the entry it lands on. A failed or exhausted step leaves the
// iterator invalid, and a failure ends it.
func (t *txObjectIterator) step(move func() error) error {
	// Stop when the iterator has already failed.
	if t.err != nil {
		return t.err
	}

	t.valid = false
	t.err = t.t.lockedLive(t.ctx, func() error {
		if err := move(); err != nil {
			return err
		}
		if !t.it.Valid() {
			return t.it.Err()
		}
		t.currKey, t.valid = t.it.Key(), true
		return nil
	})
	return t.err
}

// Close releases the iterator.
func (t *txObjectIterator) Close() {
	if t.it != nil {
		t.it.Close()
	}
	t.valid = false
	t.err = context.Canceled
}

// _ is a type assertion
var _ world.ObjectIterator = (*txObjectIterator)(nil)
