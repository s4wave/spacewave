package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
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

	// it is the underlying iterator
	it world.ObjectIterator
	// err is any error that occurred
	err error
	// currKey is the current key if valid
	currKey string
	// valid indicates if the iterator is valid
	valid bool
}

// newTxObjectIterator constructs a new tx object iterator.
func newTxObjectIterator(
	t *Tx,
	ctx context.Context,
	prefix string,
	reversed bool,
) *txObjectIterator {
	// Lock the transaction for the iterator.
	unlock, err := t.rmtx.Lock(ctx, false)
	if err != nil {
		return &txObjectIterator{err: err}
	}
	defer unlock()

	// Reject a discarded transaction.
	if t.state.discarded.Load() {
		return &txObjectIterator{err: tx.ErrDiscarded}
	}

	// Iterate the transaction objects.
	it := t.state.IterateObjects(ctx, prefix, reversed)
	return &txObjectIterator{
		t:        t,
		ctx:      ctx,
		prefix:   prefix,
		reversed: reversed,
		it:       it,
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
	// Stop when the iterator has already failed.
	if t.err != nil {
		return false
	}

	// Lock the transaction for the advance.
	unlock, err := t.t.rmtx.Lock(t.ctx, false)
	if err != nil {
		t.err = err
		t.valid = false
		return false
	}
	defer unlock()

	// Reject a discarded transaction.
	if t.t.state.discarded.Load() {
		t.err = tx.ErrDiscarded
		t.valid = false
		return false
	}

	// Advance the inner iterator.
	if !t.it.Next() || !t.it.Valid() {
		t.err = t.it.Err()
		t.valid = false
		return false
	}

	// Store the current key.
	t.currKey = t.it.Key()
	t.valid = true
	return true
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
func (t *txObjectIterator) Seek(k string) error {
	// Return an earlier seek error.
	if t.err != nil {
		return t.err
	}

	// Lock the transaction for the seek.
	unlock, err := t.t.rmtx.Lock(t.ctx, false)
	if err != nil {
		t.err = err
		t.valid = false
		return err
	}
	defer unlock()

	// Reject a discarded transaction.
	if t.t.state.discarded.Load() {
		t.err = tx.ErrDiscarded
		t.valid = false
		return t.err
	}

	// Seek the inner iterator.
	if err := t.it.Seek(k); err != nil {
		t.err = err
		t.valid = false
		return err
	}

	// Require a valid entry after the seek.
	if !t.it.Valid() {
		t.err = t.it.Err()
		t.valid = false
		return t.err
	}

	// Store the current key.
	t.currKey = t.it.Key()
	t.valid = true
	return nil
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
