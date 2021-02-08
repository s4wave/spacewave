package world_block

import (
	"context"

	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// txObjectIterator implements ObjectIterator for a Tx.
type txObjectIterator struct {
	// t is the transaction
	t *Tx
	// ctx is the context
	ctx context.Context
	// prefix is the prefix to filter by
	prefix string
	// reversed indicates if iteration is reversed
	reversed bool

	// err is any error that occurred
	err error
	// currKey is the current key if valid
	currKey string
	// valid indicates if the iterator is valid
	valid bool
}

// NewTxObjectIterator constructs a new tx object iterator.
func NewTxObjectIterator(
	t *Tx,
	ctx context.Context,
	prefix string,
	reversed bool,
) *txObjectIterator {
	return &txObjectIterator{
		t:        t,
		ctx:      ctx,
		prefix:   prefix,
		reversed: reversed,
	}
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
	if t.err != nil {
		return false
	}

	unlock, err := t.t.rmtx.Lock(t.ctx, false)
	if err != nil {
		t.err = err
		t.valid = false
		return false
	}
	defer unlock()

	if t.t.discarded {
		t.err = tx.ErrDiscarded
		t.valid = false
		return false
	}

	var valid bool
	iter := t.t.state.IterateObjects(t.ctx, t.prefix, t.reversed)
	if iter == nil {
		t.valid = false
		return false
	}
	defer iter.Close()

	if t.currKey != "" {
		if err := iter.Seek(t.currKey); err != nil {
			t.err = err
			t.valid = false
			return false
		}

		if !iter.Valid() {
			t.err = iter.Err()
			t.valid = false
			return false
		}

		// Check if Seek already moved us past the current key
		if iter.Key() == t.currKey {
			// Still on same key, need to move past it
			if !iter.Next() {
				t.err = iter.Err()
				t.valid = false
				return false
			}
		}
	}

	if iter.Valid() {
		t.currKey = iter.Key()
		t.valid = true
		return true
	}

	// Need to move to first valid entry
	if !iter.Next() || !iter.Valid() {
		t.err = iter.Err()
		t.valid = false
		return false
	}

	t.currKey = iter.Key()
	valid = true
	t.valid = valid
	return valid
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
func (t *txObjectIterator) Seek(k string) error {
	if t.err != nil {
		return t.err
	}

	unlock, err := t.t.rmtx.Lock(t.ctx, false)
	if err != nil {
		t.err = err
		t.valid = false
		return err
	}
	defer unlock()

	if t.t.discarded {
		t.err = tx.ErrDiscarded
		t.valid = false
		return t.err
	}

	iter := t.t.state.IterateObjects(t.ctx, t.prefix, t.reversed)
	if iter == nil {
		t.valid = false
		return nil
	}
	defer iter.Close()

	if err := iter.Seek(k); err != nil {
		t.err = err
		t.valid = false
		return err
	}

	if !iter.Valid() {
		t.err = iter.Err()
		t.valid = false
		return t.err
	}

	t.currKey = iter.Key()
	t.valid = true
	return nil
}

// Close releases the iterator.
func (t *txObjectIterator) Close() {
	t.valid = false
	t.err = context.Canceled
}

// _ is a type assertion
var _ world.ObjectIterator = ((*txObjectIterator)(nil))
