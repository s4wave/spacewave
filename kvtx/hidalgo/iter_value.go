package kvtx_hidalgo

import (
	"context"

	kv "github.com/hidal-go/hidalgo/kv/flat"
)

// txScanIterator implements kv.Iterator
type txScanIterator struct {
	err   error
	first bool
	value *txScanIteratorValue
	start *txScanIteratorValue
}

// txScanIteratorValue contains a value
type txScanIteratorValue struct {
	key   kv.Key
	value []byte
	next  *txScanIteratorValue
}

// Next advances an iterator.
func (i *txScanIterator) Next(ctx context.Context) bool {
	first := i.first
	if first && i.start != nil {
		i.first = false
		i.value = i.start
		return true
	}
	if i.value == nil {
		return false
	}
	i.value = i.value.next
	return i.value != nil
}

// Err returns a last encountered error.
func (i *txScanIterator) Err() error {
	return i.err
}

// Close frees resources.
func (i *txScanIterator) Close() error {
	i.value = nil
	return nil
}

// Key return current key. The value will become invalid on Next or Close.
// Caller should not modify or store the value - use Clone.
func (i *txScanIterator) Key() kv.Key {
	if i.value == nil {
		return nil
	}
	return kv.Key(i.value.key)
}

// Key return current value. The value will become invalid on Next or Close.
// Caller should not modify or store the value - use Clone.
func (i *txScanIterator) Val() kv.Value {
	if i.value == nil {
		return nil
	}
	return kv.Value(i.value.value)
}

// Reset the iterator to the starting state. Closed iterator can not reset.
func (i *txScanIterator) Reset() {
	i.value = nil
	i.first = true
	if i.start != nil {
		i.err = nil
	}
}

// _ is a type assertion
var _ kv.Iterator = ((*txScanIterator)(nil))
