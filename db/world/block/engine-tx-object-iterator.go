package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// engineTxObjectIterator implements ObjectIterator for EngineTx.
//
// performOp may replace a read snapshot's Tx between steps, so the iterator
// holds no state iterator: each step opens one on the Tx performOp supplies,
// under its read lock, and seeks back to the recorded key.
type engineTxObjectIterator struct {
	// e is the engine tx
	e *EngineTx
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

// Err returns any error that has closed the iterator.
func (e *engineTxObjectIterator) Err() error {
	return e.err
}

// Valid returns if the iterator points to a valid entry.
func (e *engineTxObjectIterator) Valid() bool {
	return e.err == nil && e.valid
}

// Key returns the current entry key, or empty string if not valid.
func (e *engineTxObjectIterator) Key() string {
	if !e.Valid() {
		return ""
	}
	return e.currKey
}

// Next advances to the next entry and returns Valid.
func (e *engineTxObjectIterator) Next() bool {
	_ = e.step(func(iter world.ObjectIterator) error {
		// Position a fresh iteration at its first entry.
		if e.currKey == "" {
			iter.Next()
			return nil
		}

		// Seek back to the current key and step past it when it still exists.
		if err := iter.Seek(e.currKey); err != nil {
			return err
		}
		if iter.Valid() && iter.Key() == e.currKey {
			iter.Next()
		}
		return nil
	})
	return e.Valid()
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
func (e *engineTxObjectIterator) Seek(k string) error {
	return e.step(func(iter world.ObjectIterator) error {
		return iter.Seek(k)
	})
}

// step opens a state iterator in the transaction, positions it with move, and
// records the entry it lands on. Each performOp attempt opens its own
// iterator, so a retry never reuses one closed by the previous attempt.
func (e *engineTxObjectIterator) step(move func(world.ObjectIterator) error) error {
	// Stop when the iterator has already failed.
	if e.err != nil {
		return e.err
	}

	e.valid = false
	e.err = e.e.performOp(e.ctx, func(tx *Tx) error {
		return tx.lockedLive(e.ctx, func() error {
			// Open a state iterator in this attempt's transaction and move it.
			iter := tx.state.IterateObjects(e.ctx, e.prefix, e.reversed)
			defer iter.Close()
			if err := move(iter); err != nil {
				return err
			}
			if !iter.Valid() {
				return iter.Err()
			}
			e.currKey, e.valid = iter.Key(), true
			return nil
		})
	})
	return e.err
}

// Close releases the iterator.
func (e *engineTxObjectIterator) Close() {
	e.valid = false
	e.err = context.Canceled
}

// _ is a type assertion
var _ world.ObjectIterator = (*engineTxObjectIterator)(nil)
