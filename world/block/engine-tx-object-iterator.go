package world_block

import (
	"context"

	"github.com/aperturerobotics/hydra/world"
)

// engineTxObjectIterator implements ObjectIterator for EngineTx.
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

// NewEngineTxObjectIterator constructs a new engine tx object iterator.
func NewEngineTxObjectIterator(
	e *EngineTx,
	ctx context.Context,
	prefix string,
	reversed bool,
) *engineTxObjectIterator {
	return &engineTxObjectIterator{
		e:        e,
		ctx:      ctx,
		prefix:   prefix,
		reversed: reversed,
	}
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
	if e.err != nil {
		return false
	}

	var valid bool
	var prevIter world.ObjectIterator
	var prevTx *Tx
	err := e.e.performOp(func(tx *Tx) error {
		var iter world.ObjectIterator
		// fast path: same txn as before
		if prevTx == tx {
			iter = prevIter
			if !iter.Next() {
				return iter.Err()
			}
			e.currKey = iter.Key()
			valid = true
			return nil
		}

		// slow path: rebuild iterator with new read txn (contents changed)
		iter = tx.IterateObjects(e.ctx, e.prefix, e.reversed)
		defer iter.Close()

		// check for error
		if err := iter.Err(); err != nil {
			return err
		}

		// save txn and iter for later
		prevTx, prevIter = tx, iter

		if e.currKey != "" {
			if err := iter.Seek(e.currKey); err != nil {
				return err
			}

			if !iter.Valid() {
				return iter.Err()
			}

			// Check if Seek already moved us past the current key
			if iter.Key() == e.currKey {
				// Still on same key, need to move past it
				if !iter.Next() {
					return iter.Err()
				}
			}
		} else {
			// Need to move to first valid entry
			if !iter.Next() {
				return iter.Err()
			}
		}

		if !iter.Valid() {
			return iter.Err()
		}

		e.currKey = iter.Key()
		valid = true
		return nil
	})
	if err != nil {
		e.err = err
		e.valid = false
		return false
	}

	e.valid = valid
	return valid
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
func (e *engineTxObjectIterator) Seek(k string) error {
	if e.err != nil {
		return e.err
	}

	var valid bool
	err := e.e.performOp(func(tx *Tx) error {
		iter := tx.IterateObjects(e.ctx, e.prefix, e.reversed)
		if iter == nil {
			return nil
		}
		defer iter.Close()

		if err := iter.Seek(k); err != nil {
			return err
		}

		if !iter.Valid() {
			return iter.Err()
		}

		e.currKey = iter.Key()
		valid = true
		return nil
	})
	if err != nil {
		e.err = err
		e.valid = false
		return err
	}

	e.valid = valid
	return nil
}

// Close releases the iterator.
func (e *engineTxObjectIterator) Close() {
	e.valid = false
	e.err = context.Canceled
}

// _ is a type assertion
var _ world.ObjectIterator = ((*engineTxObjectIterator)(nil))
