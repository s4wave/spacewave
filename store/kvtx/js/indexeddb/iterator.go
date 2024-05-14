//go:build js
// +build js

package store_kvtx_indexeddb

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/go-indexeddb/idb"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/util/jsbuf"
	"github.com/hack-pad/safejs"
)

// kvtxIterator implements a kvtx Iterator for IndexedDB.
type kvtxIterator struct {
	ctx      context.Context
	store    *idb.ObjectStore
	req      *idb.CursorRequest
	cs       *idb.Cursor
	prefix   []byte
	sort     bool
	valid    bool
	firstRun bool
	err      error
	key      []byte
	keyVal   safejs.Value
	hasVal   bool
	value    []byte
}

// BuildKvtxIterator builds an iterator for the given object store and arguments.
func BuildKvtxIterator(ctx context.Context, store *idb.ObjectStore, prefix []byte, sort, reverse bool) kvtx.Iterator {
	prefixVal, err := jsbuf.CopyBytesToJs(prefix)
	if err != nil {
		return kvtx.NewErrIterator(err)
	}

	keyRange, err := idb.NewKeyRangeLowerBound(prefixVal, false)
	if err != nil {
		return kvtx.NewErrIterator(err)
	}

	direction := idb.CursorNextUnique
	if reverse {
		direction = idb.CursorPreviousUnique
	}

	req, err := store.OpenKeyCursorRange(keyRange, direction)
	if err != nil {
		return kvtx.NewErrIterator(err)
	}

	cs, err := req.Await(ctx)
	if err != nil {
		return kvtx.NewErrIterator(err)
	}

	return &kvtxIterator{
		ctx:      ctx,
		store:    store,
		req:      req,
		cs:       cs,
		prefix:   prefix,
		sort:     sort,
		valid:    false,
		firstRun: true,
	}
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
		var err error
		it.value, err = it.fetchValue()
		if err != nil {
			return nil, err
		}
		it.hasVal = true
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

	if it.firstRun {
		it.firstRun = false
		return it.advance()
	}

	if !it.valid {
		return false
	}

	return it.advance()
}

// Seek moves the iterator to the selected key. If the key doesn't exist, it must move to the
// next smallest key greater than k.
// Seek moves the iterator to the first key >= the provided key.
// Pass nil to seek to the beginning (or end if reversed).
// Seek has two failure modes:
//   - return an error without modifying the iterator
//   - set the iterator Err to the error and return nil
func (it *kvtxIterator) Seek(k []byte) error {
	if it.err != nil {
		return it.err
	}

	keyVal, err := jsbuf.CopyBytesToJs(k)
	if err != nil {
		return err
	}

	// NOTE: ContinueKey returns an error if keyVal is < the current or keyVal is not a valid key.
	err = it.cs.ContinueKey(keyVal)
	if err != nil {
		it.err = err
		return nil
	}

	it.firstRun = false
	_ = it.advance()
	return nil
}

// Close closes the iterator.
// Note: it is not necessary to close all iterators before Discard().
func (it *kvtxIterator) Close() {
	if it.err == nil {
		it.err = context.Canceled
	}
}

// advance moves the iterator to the next key/value pair.
// Returns true if there is a next key/value pair.
func (it *kvtxIterator) advance() bool {
	cursor, err := it.req.Await(it.ctx)
	if err != nil {
		it.err = err
		return false
	}

	if cursor == nil {
		it.valid = false
		return false
	}

	keyVal, err := cursor.Key()
	if err != nil {
		it.err = err
		return false
	}

	key, err := jsbuf.CopyBytesToGo(keyVal)
	if err != nil {
		it.err = err
		return false
	}

	if !it.sort && !bytes.HasPrefix(key, it.prefix) {
		it.valid = false
		return false
	}

	it.key = key
	it.keyVal = keyVal
	it.value = nil
	it.hasVal = false
	it.valid = true
	return true
}

// fetchValue fetches the current value from the db.
func (it *kvtxIterator) fetchValue() ([]byte, error) {
	getReq, err := it.store.Get(it.keyVal)
	if err != nil {
		return nil, err
	}

	getVal, err := getReq.Await(it.ctx)
	if err != nil {
		return nil, err
	}

	value, err := jsbuf.CopyBytesToGo(getVal)
	if err != nil {
		return nil, err
	}

	return value, nil
}

// _ is a type assertion
var _ kvtx.Iterator = (*kvtxIterator)(nil)
