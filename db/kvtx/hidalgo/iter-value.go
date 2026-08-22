package kvtx_hidalgo

import (
	"context"

	kv "github.com/aperturerobotics/cayley/kv/flat"
	"github.com/s4wave/spacewave/db/kvtx"
)

// txScanIterator implements kv.Iterator
type txScanIterator struct {
	ctx     context.Context
	tx      kvtx.Tx
	prefix  []byte
	iter    kvtx.Iterator
	err     error
	started bool
	key     kv.Key
	value   kv.Value
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
	if i.err != nil {
		return false
	}
	if err := ctx.Err(); err != nil {
		i.err = err
		return false
	}
	i.key = nil
	i.value = nil

	iter := i.getIterator()
	if iter == nil {
		return false
	}

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

	if !iter.Valid() {
		i.err = iter.Err()
		return false
	}
	value, err := iter.ValueCopy(nil)
	if err != nil {
		i.err = err
		return false
	}
	i.key = kv.Key(iter.Key()).Clone()
	i.value = kv.Value(value).Clone()
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
	i.key = nil
	i.value = nil
	i.err = nil
	i.started = false
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
