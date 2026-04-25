//go:build js && wasm

package store

import (
	"bytes"
	"context"
	"sort"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
	opfs "github.com/s4wave/spacewave/prototypes/opfs/go-opfs"
)

// opfsTx implements kvtx.Tx backed by OPFS flat files.
type opfsTx struct {
	store     *Store
	write     bool
	discarded atomic.Bool
}

func newTx(s *Store, write bool) *opfsTx {
	return &opfsTx{store: s, write: write}
}

// getShardDir returns the shard directory for an encoded key, creating if write.
func (t *opfsTx) getShardDir(encoded string) (*opfs.DirectoryHandle, error) {
	shard := shardPrefix(encoded)
	return t.store.data.GetDirectoryHandle(shard, t.write)
}

// Size returns the number of keys in the store.
func (t *opfsTx) Size(ctx context.Context) (uint64, error) {
	if t.discarded.Load() {
		return 0, kvtx.ErrDiscarded
	}
	// Count all files across all shard dirs.
	entries, err := t.store.data.Entries()
	if err != nil {
		return 0, err
	}
	var count uint64
	for _, e := range entries {
		if e.Kind != "directory" {
			continue
		}
		dir := e.AsDirectoryHandle()
		files, err := dir.Entries()
		if err != nil {
			return 0, err
		}
		for _, f := range files {
			if f.Kind == "file" {
				count++
			}
		}
	}
	return count, nil
}

// Get returns the value for a key.
func (t *opfsTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if t.discarded.Load() {
		return nil, false, kvtx.ErrDiscarded
	}
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}

	encoded := encodeKey(key)
	dir, err := t.getShardDir(encoded)
	if err != nil {
		// Directory not found means key not found.
		return nil, false, nil
	}

	fh, err := dir.GetFileHandle(encoded, false)
	if err != nil {
		// File not found.
		return nil, false, nil
	}

	data, err := fh.ReadFile()
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// Exists checks if a key exists.
func (t *opfsTx) Exists(ctx context.Context, key []byte) (bool, error) {
	if t.discarded.Load() {
		return false, kvtx.ErrDiscarded
	}
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}

	encoded := encodeKey(key)
	dir, err := t.getShardDir(encoded)
	if err != nil {
		return false, nil
	}

	_, err = dir.GetFileHandle(encoded, false)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// Set writes a key-value pair.
func (t *opfsTx) Set(ctx context.Context, key, value []byte) error {
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	encoded := encodeKey(key)
	dir, err := t.getShardDir(encoded)
	if err != nil {
		return err
	}

	fh, err := dir.GetFileHandle(encoded, true)
	if err != nil {
		return err
	}

	ops, err := fh.OpenFileOps()
	if err != nil {
		return err
	}

	// Read old size for tally adjustment.
	fi, _ := ops.Stat()
	var oldSize int64
	if fi != nil {
		oldSize = fi.Size()
	}

	// Truncate first to clear old data.
	if err := ops.Truncate(0); err != nil {
		_ = ops.Close()
		return err
	}

	if len(value) > 0 {
		_, err = ops.Write(value)
		if err != nil {
			_ = ops.Close()
			return err
		}
	}

	if err := ops.Flush(); err != nil {
		_ = ops.Close()
		return err
	}

	// Update tally: subtract old, add new.
	t.store.mu.Lock()
	t.store.tally -= uint64(oldSize)
	t.store.tally += uint64(len(value))
	t.store.mu.Unlock()

	return ops.Close()
}

// Delete removes a key.
func (t *opfsTx) Delete(ctx context.Context, key []byte) error {
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	encoded := encodeKey(key)
	dir, err := t.getShardDir(encoded)
	if err != nil {
		return nil // not found, nothing to delete
	}

	// Read old size for tally adjustment before removing.
	fh, fhErr := dir.GetFileHandle(encoded, false)
	if fhErr == nil {
		data, readErr := fh.ReadFile()
		if readErr == nil {
			t.store.mu.Lock()
			t.store.tally -= uint64(len(data))
			t.store.mu.Unlock()
		}
	}

	return dir.RemoveEntry(encoded, false)
}

// ScanPrefix iterates over keys with a prefix, calling cb for each.
func (t *opfsTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}

	it := t.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		val, err := it.Value()
		if err != nil {
			return err
		}
		if err := cb(it.Key(), val); err != nil {
			return err
		}
	}
	return it.Err()
}

// ScanPrefixKeys iterates over keys with a prefix.
func (t *opfsTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}

	it := t.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		if err := cb(it.Key()); err != nil {
			return err
		}
	}
	return it.Err()
}

// Iterate returns an iterator over keys with the given prefix.
func (t *opfsTx) Iterate(ctx context.Context, prefix []byte, doSort, reverse bool) kvtx.Iterator {
	if t.discarded.Load() {
		return kvtx.NewErrIterator(kvtx.ErrDiscarded)
	}

	// Collect all matching keys.
	type kv struct {
		key []byte
		enc string
	}
	var items []kv

	shards, err := t.store.data.Entries()
	if err != nil {
		return kvtx.NewErrIterator(err)
	}

	for _, shard := range shards {
		if shard.Kind != "directory" {
			continue
		}
		dir := shard.AsDirectoryHandle()
		files, err := dir.Entries()
		if err != nil {
			return kvtx.NewErrIterator(err)
		}
		for _, f := range files {
			if f.Kind != "file" {
				continue
			}
			key, err := decodeKey(f.Name)
			if err != nil {
				continue
			}
			if len(prefix) > 0 && !bytes.HasPrefix(key, prefix) {
				continue
			}
			items = append(items, kv{key: key, enc: f.Name})
		}
	}

	if doSort {
		sort.Slice(items, func(i, j int) bool {
			return bytes.Compare(items[i].key, items[j].key) < 0
		})
		if reverse {
			for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	keys := make([][]byte, len(items))
	encodedNames := make([]string, len(items))
	for i, item := range items {
		keys[i] = item.key
		encodedNames[i] = item.enc
	}
	return &opfsIterator{
		tx:    t,
		keys:  keys,
		names: encodedNames,
		pos:   -1,
	}
}

// Commit commits the transaction.
func (t *opfsTx) Commit(ctx context.Context) error {
	if t.discarded.Swap(true) {
		return kvtx.ErrDiscarded
	}
	// OPFS writes are already persisted via flush in Set.
	return nil
}

// Discard discards the transaction.
func (t *opfsTx) Discard() {
	t.discarded.Store(true)
}

// _ is a type assertion
var _ kvtx.Tx = (*opfsTx)(nil)
