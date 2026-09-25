//go:build js

package volume_idb

import (
	"bytes"
	"context"
	"syscall/js"

	"github.com/s4wave/spacewave/db/volume/records"
)

const (
	// recordStore holds the records under their binary keys.
	recordStore = "records"
	// syncStore holds one record an empty durable commit rewrites, so the
	// commit has a write to make durable.
	syncStore = "sync"
)

// Store is a records.Store on one IndexedDB database.
type Store struct {
	// db is the database connection.
	db js.Value
}

// OpenStore opens the record store database name, creating it when absent.
func OpenStore(ctx context.Context, name string) (*Store, error) {
	db, err := openDB(name, recordStore, syncStore)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// Get reads the records of keys in one transaction.
func (s *Store) Get(ctx context.Context, keys [][]byte) ([][]byte, error) {
	tx := transaction(s.db, false, false, recordStore)
	store := tx.Call("objectStore", recordStore)
	reqs := make([]js.Value, len(keys))
	for i, key := range keys {
		reqs[i] = store.Call("get", toJS(key))
	}
	if err := complete(tx); err != nil {
		return nil, err
	}
	out := make([][]byte, len(keys))
	for i, req := range reqs {
		if v := req.Get("result"); !v.IsUndefined() {
			out[i] = toGo(v)
		}
	}
	return out, nil
}

// Has reports which keys have records in one transaction.
func (s *Store) Has(ctx context.Context, keys [][]byte) ([]bool, error) {
	tx := transaction(s.db, false, false, recordStore)
	store := tx.Call("objectStore", recordStore)
	reqs := make([]js.Value, len(keys))
	for i, key := range keys {
		reqs[i] = store.Call("getKey", toJS(key))
	}
	if err := complete(tx); err != nil {
		return nil, err
	}
	out := make([]bool, len(keys))
	for i, req := range reqs {
		out[i] = !req.Get("result").IsUndefined()
	}
	return out, nil
}

// Scan reads every record under prefix in one transaction, then calls fn
// with each in key order.
func (s *Store) Scan(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error {
	// Read the keys and values of the prefix range.
	tx := transaction(s.db, false, false, recordStore)
	store := tx.Call("objectStore", recordStore)
	rng := prefixRange(prefix)
	keysReq := store.Call("getAllKeys", rng)
	valuesReq := store.Call("getAll", rng)
	if err := complete(tx); err != nil {
		return err
	}

	// Visit them in order.
	keys, values := keysReq.Get("result"), valuesReq.Get("result")
	for i := range keys.Length() {
		if err := fn(toGo(keys.Index(i)), toGo(values.Index(i))); err != nil {
			return err
		}
	}
	return nil
}

// Commit applies ops in one transaction, strict when durable is set.
func (s *Store) Commit(ctx context.Context, ops []records.Op, durable bool) error {
	tx := transaction(s.db, true, durable, recordStore, syncStore)
	store := tx.Call("objectStore", recordStore)
	for _, op := range ops {
		if op.Delete {
			store.Call("delete", toJS(op.Key))
			continue
		}
		store.Call("put", toJS(op.Value), toJS(op.Key))
	}
	if len(ops) == 0 && durable {
		tx.Call("objectStore", syncStore).Call("put", 0, 0)
	}
	return complete(tx)
}

// Close closes the database connection.
func (s *Store) Close() error {
	s.db.Call("close")
	return nil
}

// prefixRange returns the key range of prefix, or undefined for every key.
func prefixRange(prefix []byte) js.Value {
	keyRange := js.Global().Get("IDBKeyRange")
	if len(prefix) == 0 {
		return js.Undefined()
	}
	end := prefixEnd(prefix)
	if end == nil {
		return keyRange.Call("lowerBound", toJS(prefix))
	}
	return keyRange.Call("bound", toJS(prefix), toJS(end), false, true)
}

// prefixEnd returns the least key greater than every key with prefix, or nil
// when prefix is all 0xff.
func prefixEnd(prefix []byte) []byte {
	end := bytes.Clone(prefix)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] != 0xff {
			end[i]++
			return end[:i+1]
		}
	}
	return nil
}

// _ checks that Store is a records.Store.
var _ records.Store = (*Store)(nil)
