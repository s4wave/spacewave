package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// lockedObjectIterator implements ObjectIterator for Tx and EngineTx over a
// WorldState iterator that each step reaches under the transaction's read lock.
//
// A Tx holds one state iterator open across steps. performOp may replace an
// EngineTx read snapshot's Tx between steps, so an EngineTx iterator holds none:
// each step opens one on the Tx performOp supplies and seeks back to the
// recorded key.
type lockedObjectIterator struct {
	// run calls fn with a state iterator under the read lock.
	run func(fn func(world.ObjectIterator) error) error
	// reopens is set when run opens a fresh state iterator each step.
	reopens bool
	// held is the state iterator a Tx holds open across steps.
	held world.ObjectIterator

	// err is any error that occurred
	err error
	// currKey is the current key if valid
	currKey string
	// valid indicates if the iterator is valid
	valid bool
}

// Err returns any error that has closed the iterator.
func (it *lockedObjectIterator) Err() error {
	return it.err
}

// Valid returns if the iterator points to a valid entry.
func (it *lockedObjectIterator) Valid() bool {
	return it.err == nil && it.valid
}

// Key returns the current entry key, or empty string if not valid.
func (it *lockedObjectIterator) Key() string {
	if !it.Valid() {
		return ""
	}
	return it.currKey
}

// Next advances to the next entry and returns Valid.
func (it *lockedObjectIterator) Next() bool {
	_ = it.step(func(iter world.ObjectIterator) error {
		// A held iterator, or a fresh one with no entry yet, steps forward.
		if !it.reopens || it.currKey == "" {
			iter.Next()
			return nil
		}

		// Seek back to the current key and step past it when it still exists.
		if err := iter.Seek(it.currKey); err != nil {
			return err
		}
		if iter.Valid() && iter.Key() == it.currKey {
			iter.Next()
		}
		return nil
	})
	return it.Valid()
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
func (it *lockedObjectIterator) Seek(k string) error {
	return it.step(func(iter world.ObjectIterator) error {
		return iter.Seek(k)
	})
}

// step positions a state iterator with move under the read lock and records
// the entry it lands on. A failed or exhausted step leaves the iterator
// invalid, and a failure ends it.
func (it *lockedObjectIterator) step(move func(world.ObjectIterator) error) error {
	// Stop when the iterator has already failed.
	if it.err != nil {
		return it.err
	}

	it.valid = false
	it.err = it.run(func(iter world.ObjectIterator) error {
		if err := move(iter); err != nil {
			return err
		}
		if !iter.Valid() {
			return iter.Err()
		}
		it.currKey, it.valid = iter.Key(), true
		return nil
	})
	return it.err
}

// Close releases the iterator.
func (it *lockedObjectIterator) Close() {
	if it.held != nil {
		it.held.Close()
	}
	it.valid = false
	it.err = context.Canceled
}

// _ is a type assertion
var _ world.ObjectIterator = (*lockedObjectIterator)(nil)
