//go:build js
// +build js

package store_kvtx_indexeddb

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/go-indexeddb/durable"
	"github.com/aperturerobotics/go-indexeddb/idb"
	"github.com/hack-pad/safejs"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/util/jsbuf"
)

// kvtxIterator implements a kvtx Iterator for IndexedDB.
type kvtxIterator struct {
	ctx   context.Context
	store *durable.DurableObjectStore
	dir   idb.CursorDirection

	// if prefix is set, upper is set
	prefix    []byte
	prefixVal safejs.Value
	upperVal  safejs.Value // upper is the upper bound Uint8Array

	// valid indicates the current position is valid
	valid bool
	// err contains any final error for the iterator
	err error

	// key contains the current key we are iterating
	key    []byte
	keyVal safejs.Value
	// hasVal indicates we have fetched the value
	hasVal bool
	value  []byte

	// txn contains the current txn, if txn changes, the below fields are cleared.
	txn      *idb.Transaction
	req      *idb.CursorRequest
	cs       *idb.Cursor
	firstRun bool
}

// BuildKvtxIterator builds an iterator for the given object store and arguments.
func BuildKvtxIterator(ctx context.Context, store *durable.DurableObjectStore, prefix []byte, reverse bool) kvtx.Iterator {
	dir := idb.CursorNextUnique
	if reverse {
		dir = idb.CursorPreviousUnique
	}

	it := &kvtxIterator{
		ctx:      ctx,
		store:    store,
		dir:      dir,
		firstRun: true,
	}

	if len(prefix) != 0 {
		// Append the maximum value for a byte to the list to get the upper bound.
		// This effectively restricts to the prefix.
		prefixUpperBound := make([]byte, len(prefix)+1)
		prefixUpperBound[len(prefixUpperBound)-1] = 255
		copy(prefixUpperBound, prefix)

		// Keep upper bound in the prefix slice, just out of the bounds.
		it.prefix = prefixUpperBound[:len(prefix)]

		upperBoundVal, err := jsbuf.CopyBytesToJs(prefixUpperBound)
		if err != nil {
			return kvtx.NewErrIterator(err)
		}

		// remove the last index to get the lower bound w/o alloc
		prefixVal, err := upperBoundVal.Call("subarray", 0, len(prefix))
		if err != nil {
			return kvtx.NewErrIterator(err)
		}

		it.upperVal = upperBoundVal
		it.prefixVal = prefixVal
	}

	return it
}

// performOp performs an operation with retry in case the txn was auto-committed.
//
// note: cs will be nil if there are no further results
func (it *kvtxIterator) performOp(
	ctx context.Context,
	cb func(
		txn *idb.Transaction,
		store *idb.ObjectStore,
		req *idb.CursorRequest,
		cs *idb.Cursor,
	) error,
) error {
	if it.err != nil {
		return it.err
	}

	err := it.store.StoreWithRetry(func(txn *idb.Transaction, store *idb.ObjectStore) error {
		it.setTxn(txn)

		// if necessary, open the cursor again.
		var err error
		req := it.req
		if req == nil {
			var keyRng *idb.KeyRange
			if len(it.key) != 0 {
				// if we are iterating, resume where we left off
				if len(it.prefix) != 0 {
					// Compare key with prefix bounds
					keyVal := it.keyVal
					if it.dir == idb.CursorPreviousUnique {
						// For reverse iteration with prefix:
						// If key > upper, use upper as the key
						// If key < prefix, use prefix as the key
						// Lower bound (prefix) is closed to include prefix
						// Upper bound (current key) is closed to include current key
						if bytes.Compare(it.key, it.prefix) < 0 {
							keyVal = it.prefixVal
						}
						keyRng, err = idb.NewKeyRangeBound(it.prefixVal, keyVal, false, false)
					} else {
						// For forward iteration with prefix:
						// If key < prefix, use prefix as the key
						// If key > upper, use upper as the key
						// Lower bound (current key) is closed to include current key
						// Upper bound is open to exclude anything >= upper
						if bytes.Compare(it.key, it.prefix) < 0 {
							keyVal = it.prefixVal
						} else if bytes.Compare(it.key, it.prefix[:len(it.prefix)+1]) >= 0 {
							keyVal = it.prefixVal
						}
						keyRng, err = idb.NewKeyRangeBound(keyVal, it.upperVal, false, true)
					}
				} else {
					if it.dir == idb.CursorPreviousUnique {
						// For reverse iteration without prefix:
						// Upper bound is closed to include current key
						keyRng, err = idb.NewKeyRangeUpperBound(it.keyVal, false)
					} else {
						// For forward iteration without prefix:
						// Lower bound is closed to include current key
						keyRng, err = idb.NewKeyRangeLowerBound(it.keyVal, false)
					}
				}
			} else if len(it.prefix) != 0 {
				// If we have a prefix but no current key (first iteration):
				// Lower bound (prefix) is closed to include the prefix
				// Upper bound is open to exclude anything >= upper
				// the prefix exactly (all bytes equal to prefix). The upper bound is
				// prefix + 0xFF which ensures we get all keys starting with prefix.
				keyRng, err = idb.NewKeyRangeBound(it.prefixVal, it.upperVal, false, true)
			}
			if err != nil {
				return err
			}

			// if we have a key range, use it.
			if keyRng != nil {
				req, err = store.OpenKeyCursorRange(keyRng, it.dir)
			} else {
				req, err = store.OpenKeyCursor(it.dir)
			}
			if err != nil {
				return err
			}

			it.req = req
			it.cs = nil
		}

		// await the cursor
		cs := it.cs
		if cs == nil {
			cs, err = req.Await(ctx)
			if err != nil {
				return err
			}
			it.cs = cs
		}

		// call the callback
		return cb(txn, store, req, cs)
	})
	if err != nil && it.err == nil {
		it.err = err
	}
	return err
}

// setTxn updates the txn field clearing the state if the txn changed
func (it *kvtxIterator) setTxn(txn *idb.Transaction) {
	if it.txn != nil && it.txn == txn {
		return
	}
	it.txn = txn
	it.req = nil
	it.cs = nil
	it.firstRun = true
}

// Err returns any error that has closed the iterator.
// May return context.Canceled if closed.
func (it *kvtxIterator) Err() error {
	return it.err
}

// Valid returns if the iterator points to a valid entry.
//
// If err is set, returns false.
func (it *kvtxIterator) Valid() bool {
	return it.valid && it.err == nil
}

// Key returns the current entry key, or nil if not valid.
func (it *kvtxIterator) Key() []byte {
	if !it.Valid() {
		return nil
	}
	return it.key
}

// Value returns the current entry value, or nil if not valid.
func (it *kvtxIterator) Value() ([]byte, error) {
	if !it.Valid() {
		return nil, it.err
	}
	if !it.hasVal {
		err := it.performOp(it.ctx, func(txn *idb.Transaction, store *idb.ObjectStore, req *idb.CursorRequest, cs *idb.Cursor) error {
			var err error
			it.value, err = it.fetchValue()
			if err != nil {
				return err
			}
			it.hasVal = len(it.value) != 0
			if !it.hasVal {
				// this key was not found.
				// next time we run Next() a new cursor will be constructed starting at the next key after this key.
				// TODO: should we return an error here?
				it.cs = nil
				it.req = nil
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return it.value, nil
}

// ValueCopy copies the value to the given byte slice and returns it.
// If the slice is not big enough (cap), it must create a new one and return it.
func (it *kvtxIterator) ValueCopy(dst []byte) ([]byte, error) {
	if !it.Valid() {
		return nil, it.err
	}
	value, err := it.Value()
	if err != nil {
		return nil, err
	}
	return append(dst[:0], value...), nil
}

// Next moves the iterator to the next item.
// Next advances to the next entry and returns Valid.
func (it *kvtxIterator) Next() bool {
	if it.err != nil {
		return false
	}

	var valid bool
	err := it.performOp(
		it.ctx,
		func(txn *idb.Transaction, store *idb.ObjectStore, req *idb.CursorRequest, cs *idb.Cursor) error {
			var err error
			valid, err = it.initCursorMaybeContinue(cs)
			return err
		},
	)
	if err != nil {
		if it.err == nil {
			it.err = err
		}
		return false
	}

	return valid
}

// Seek moves the iterator to the selected key. If the key doesn't exist, it must move to the
// next smallest key greater than k.
// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
// Pass nil to seek to the beginning (or end if reversed).
// Seek has two failure modes:
//   - return an error without modifying the iterator
//   - set the iterator Err to the error and return nil
func (it *kvtxIterator) Seek(k []byte) error {
	if it.err != nil {
		return it.err
	}

	// clear the cursor and request, we will build a new one.
	if len(k) == 0 {
		it.key = nil
		it.keyVal = safejs.Undefined()
	} else {
		it.key = bytes.Clone(k)
		it.keyVal, it.err = jsbuf.CopyBytesToJs(k)
		if it.err != nil {
			return it.err
		}
	}

	it.req = nil
	it.cs = nil
	it.firstRun = true
	it.valid = false
	it.hasVal = false
	it.value = nil

	// assert there is no error and set valid properly
	_ = it.Next() // firstRun = true => does not advance the cursor
	return it.err
}

// Close closes the iterator.
// Note: it is not necessary to close all iterators before Discard().
func (it *kvtxIterator) Close() {
	if it.err == nil {
		it.err = context.Canceled
	}
}

// fetchValue fetches the current value from the db.
func (it *kvtxIterator) fetchValue() ([]byte, error) {
	getVal, err := it.store.Get(it.ctx, it.keyVal)
	if err != nil {
		return nil, err
	}

	if getVal.IsUndefined() {
		return nil, nil
	}

	value, err := jsbuf.CopyBytesToGo(getVal)
	if err != nil {
		return nil, err
	}

	return value, nil
}

// initCursorMaybeContinue moves the cursor to the next position and updates the key.
// Returns false if there is no next position.
// this is intended to be called within performOp callback
// if firstRun, we skip calling Continue().
func (it *kvtxIterator) initCursorMaybeContinue(cs *idb.Cursor) (bool, error) {
	// clear the existing stored value if applicable
	it.hasVal = false
	it.value = nil

	if cs == nil {
		// no further results
		it.key = nil
		it.keyVal = safejs.Undefined()
		it.valid = false
		return false, nil
	}

	if it.firstRun {
		// On first run we don't advance the cursor since it's already
		// positioned at the first matching key from OpenKeyCursor
		it.firstRun = false
	} else {
		// Continue advances to the next key that matches our range
		// We must clear the cursor since the transaction may have changed
		if err := cs.Continue(); err != nil {
			return false, err
		}
		it.cs = nil

		// Await the cursor result - this may return nil if we've reached
		// the end of the range or if the key is outside our bounds
		var err error
		cs, err = it.req.AwaitCursor(it.ctx)
		if err != nil {
			return false, err
		}
		if cs == nil {
			// No more results in our range
			it.key = nil
			it.keyVal = safejs.Undefined()
			it.valid = false
			return false, nil
		}
		it.cs = cs
	}

	keyVal, err := cs.Key()
	if err != nil {
		return false, err
	}
	key, err := jsbuf.CopyBytesToGo(keyVal)
	if err != nil {
		return false, err
	}
	it.key = key
	it.keyVal = keyVal
	it.valid = true
	return true, nil
}

// _ is a type assertion
var _ kvtx.Iterator = (*kvtxIterator)(nil)
