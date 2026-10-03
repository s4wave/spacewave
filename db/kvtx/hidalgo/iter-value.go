package kvtx_hidalgo

import (
	"context"

	kv "github.com/aperturerobotics/cayley/kv/flat"
	"github.com/s4wave/spacewave/db/kvtx"
)

// txScanIterator implements kv.Iterator
type txScanIterator struct {
	// ctx is the context the iterator was created with.
	ctx context.Context
	// tx is the transaction the iterator reads from.
	tx kvtx.Tx
	// prefix restricts iteration to keys starting with it.
	prefix []byte
	// iter is the lazily created underlying iterator.
	iter kvtx.Iterator
	// err is the first error encountered during iteration.
	err error
	// started indicates whether the underlying iterator has begun traversing.
	started bool
	// beforeStart fences buffered writes before a new or reset lazy snapshot.
	beforeStart func(context.Context) error
	// key is the current key.
	key kv.Key
	// value is the current value.
	value kv.Value
}

func newTxScanIterator(ctx context.Context, tx kvtx.Tx, prefix []byte) *txScanIterator {
	return &txScanIterator{
		ctx:    ctx,
		tx:     tx,
		prefix: prefix,
	}
}

// Next advances an iterator.
func (i *txScanIterator) Next(ctx context.Context) bool {
	// Stop after an earlier error or a canceled context.
	if i.err != nil {
		return false
	}
	if err := ctx.Err(); err != nil {
		i.err = err
		return false
	}

	// Clear the previous entry.
	i.key = nil
	i.value = nil

	// Run the start hook once, then resolve the underlying iterator.
	if !i.started && i.beforeStart != nil {
		if err := i.beforeStart(ctx); err != nil {
			i.err = err
			return false
		}
	}
	iter := i.getIterator()
	if iter == nil {
		return false
	}

	// Seek to the first entry on the first call and advance afterward.
	if !i.started {
		i.started = true
		if err := iter.Seek(nil); err != nil {
			i.err = err
			return false
		}
	} else if !iter.Next() {
		i.err = iter.Err()
		return false
	}

	// Copy the value, which ValueCopy into a nil buffer already owns.
	if !iter.Valid() {
		i.err = iter.Err()
		return false
	}
	value, err := iter.ValueCopy(nil)
	if err != nil {
		i.err = err
		return false
	}

	// Keep an owned key and the owned value.
	i.key = kv.Key(iter.Key()).Clone()
	i.value = kv.Value(value)
	return true
}

// Err returns a last encountered error.
func (i *txScanIterator) Err() error {
	return i.err
}

// Key returns the current key. The value becomes invalid on Next or Close.
// Caller should not modify or store the value; use Clone.
func (i *txScanIterator) Key() kv.Key {
	return i.key
}

// Val returns the current value. The value becomes invalid on Next or Close.
// Caller should not modify or store the value; use Clone.
func (i *txScanIterator) Val() kv.Value {
	return i.value
}

// Close frees resources.
func (i *txScanIterator) Close() error {
	i.key = nil
	i.value = nil
	if i.iter != nil {
		i.iter.Close()
		i.iter = nil
	}
	return nil
}

// Reset resets the iterator to the starting state. Closed iterators cannot reset.
func (i *txScanIterator) Reset() {
	// Clear the scan snapshot and its previous error before restarting.
	i.key = nil
	i.value = nil
	i.err = nil
	i.started = false

	// Release the backend iterator so the next scan opens a fresh snapshot.
	if i.iter != nil {
		i.iter.Close()
		i.iter = nil
	}
}

func (i *txScanIterator) getIterator() kvtx.Iterator {
	if i.iter != nil {
		return i.iter
	}
	i.iter = i.tx.Iterate(i.ctx, i.prefix, true, false)
	return i.iter
}

// _ is a type assertion
var _ kv.Iterator = (*txScanIterator)(nil)
