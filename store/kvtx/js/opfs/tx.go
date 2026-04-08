//go:build js

package store_kvtx_opfs

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"sort"
	"sync/atomic"
	"syscall/js"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/aperturerobotics/hydra/opfs"
	"github.com/pkg/errors"
)

// pendingMarker is the filename used to mark an in-flight write.
const pendingMarker = ".pending"

// Tx is an OPFS transaction.
type Tx struct {
	store     *Store
	write     bool
	discarded atomic.Bool
	release   func()

	// Write buffer (write tx only).
	sets    map[string][]byte
	deletes map[string]struct{}
}

func (t *Tx) checkActive() error {
	if t.discarded.Load() {
		return kvtx.ErrDiscarded
	}
	return nil
}

// getShardDir returns the shard directory handle, creating it if create is true.
func (t *Tx) getShardDir(shard string, create bool) (js.Value, error) {
	return opfs.GetDirectory(t.store.root, shard, create)
}

// openFile opens an existing file for reading.
func (t *Tx) openFile(dir js.Value, name string) (fs.File, error) {
	if t.store.sync {
		return opfs.OpenSyncFile(dir, name)
	}
	return opfs.OpenAsyncFile(dir, name)
}

// readFile reads a file's full contents.
func (t *Tx) readFile(dir js.Value, name string) ([]byte, error) {
	f, err := t.openFile(dir, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// writeFile writes a file's full contents.
func (t *Tx) writeFile(dir js.Value, name string, data []byte) error {
	if t.store.sync {
		f, err := opfs.CreateSyncFile(dir, name)
		if err != nil {
			return err
		}
		defer f.Close()
		f.Truncate(0)
		_, err = f.Write(data)
		f.Flush()
		return err
	}
	return opfs.WriteFile(dir, name, data)
}

// Get returns values for a key.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	if err := t.checkActive(); err != nil {
		return nil, false, err
	}
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}

	encoded := encodeKey(key)

	// Check write buffer first.
	if t.write {
		if _, deleted := t.deletes[encoded]; deleted {
			return nil, false, nil
		}
		if val, ok := t.sets[encoded]; ok {
			return append([]byte(nil), val...), true, nil
		}
	}

	shard := shardPrefix(encoded)
	shardDir, err := t.getShardDir(shard, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	data, err := t.readFile(shardDir, encoded)
	if err != nil {
		if opfs.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

// Set sets the value of a key.
func (t *Tx) Set(ctx context.Context, key, value []byte) error {
	if err := t.checkActive(); err != nil {
		return err
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	encoded := encodeKey(key)
	t.sets[encoded] = append([]byte(nil), value...)
	delete(t.deletes, encoded)
	return nil
}

// Delete deletes a key.
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	if err := t.checkActive(); err != nil {
		return err
	}
	if !t.write {
		return kvtx.ErrNotWrite
	}
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	encoded := encodeKey(key)
	t.deletes[encoded] = struct{}{}
	delete(t.sets, encoded)
	return nil
}

// Exists checks if a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	if err := t.checkActive(); err != nil {
		return false, err
	}
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}

	encoded := encodeKey(key)

	if t.write {
		if _, deleted := t.deletes[encoded]; deleted {
			return false, nil
		}
		if _, ok := t.sets[encoded]; ok {
			return true, nil
		}
	}

	shard := shardPrefix(encoded)
	shardDir, err := t.getShardDir(shard, false)
	if err != nil {
		if opfs.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return opfs.FileExists(shardDir, encoded)
}

// Size returns the number of keys in the store.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	if err := t.checkActive(); err != nil {
		return 0, err
	}

	// Count OPFS entries, tracking seen keys for write buffer dedup.
	var count uint64
	var seen map[string]struct{}
	if t.write {
		seen = make(map[string]struct{})
	}

	shardNames, err := opfs.ListDirectory(t.store.root)
	if err != nil {
		return 0, err
	}
	for _, shard := range shardNames {
		if len(shard) != 2 || shard == pendingMarker {
			continue
		}
		shardDir, err := t.getShardDir(shard, false)
		if err != nil {
			if opfs.IsNotFound(err) {
				continue
			}
			return 0, err
		}
		entries, err := opfs.ListDirectory(shardDir)
		if err != nil {
			return 0, err
		}
		for _, name := range entries {
			if t.write {
				if _, deleted := t.deletes[name]; deleted {
					continue
				}
				seen[name] = struct{}{}
			}
			count++
		}
	}

	// Add buffered sets not already on disk.
	if t.write {
		for encoded := range t.sets {
			if _, deleted := t.deletes[encoded]; deleted {
				continue
			}
			if _, onDisk := seen[encoded]; !onDisk {
				count++
			}
		}
	}

	return count, nil
}

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	if err := t.checkActive(); err != nil {
		return err
	}

	entries, err := t.collectEntries(prefix, false)
	if err != nil {
		return err
	}
	for _, e := range entries {
		val := e.value
		if val == nil {
			// Check write buffer.
			if t.write {
				val = t.sets[e.encoded]
			}
			// Load from OPFS if not in buffer.
			if val == nil {
				shard := shardPrefix(e.encoded)
				shardDir, shardErr := t.getShardDir(shard, false)
				if shardErr != nil {
					return shardErr
				}
				val, err = t.readFile(shardDir, e.encoded)
				if err != nil {
					return err
				}
			}
		}
		if err := cb(e.key, val); err != nil {
			return err
		}
	}
	return nil
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	if err := t.checkActive(); err != nil {
		return err
	}

	entries, err := t.collectEntries(prefix, false)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := cb(e.key); err != nil {
			return err
		}
	}
	return nil
}

// Iterate returns an iterator with a given key prefix.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sortIter, reverse bool) kvtx.Iterator {
	if err := t.checkActive(); err != nil {
		return kvtx.NewErrIterator(err)
	}

	entries, err := t.collectEntries(prefix, false)
	if err != nil {
		return kvtx.NewErrIterator(err)
	}

	// Entries stay in ascending order. Direction is handled by Next/Seek.
	startPos := -1
	if reverse {
		startPos = len(entries)
	}

	return &Iterator{
		tx:      t,
		entries: entries,
		pos:     startPos,
		reverse: reverse,
	}
}

// kvEntry holds a collected key for scan/iterate.
type kvEntry struct {
	key     []byte
	encoded string
	value   []byte // nil if not loaded
}

// collectEntries returns all keys matching the prefix, sorted.
// If loadValues is true, values are read from OPFS.
//
// b58 encoding does not preserve prefix relationships, so all shards
// are scanned and filtering is performed on decoded keys.
func (t *Tx) collectEntries(prefix []byte, loadValues bool) ([]kvEntry, error) {
	seen := make(map[string]struct{})
	var entries []kvEntry

	// Scan all shard directories.
	shardNames, err := opfs.ListDirectory(t.store.root)
	if err != nil {
		return nil, err
	}

	for _, shard := range shardNames {
		if len(shard) != 2 {
			continue
		}
		shardDir, err := t.getShardDir(shard, false)
		if err != nil {
			if opfs.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		names, err := opfs.ListDirectory(shardDir)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			if t.write {
				if _, deleted := t.deletes[name]; deleted {
					continue
				}
			}
			key, err := decodeKey(name)
			if err != nil {
				continue
			}
			if len(prefix) > 0 && !bytes.HasPrefix(key, prefix) {
				continue
			}
			seen[name] = struct{}{}

			var val []byte
			if t.write {
				if v, ok := t.sets[name]; ok {
					val = v
				}
			}
			if val == nil && loadValues {
				val, err = t.readFile(shardDir, name)
				if err != nil {
					if opfs.IsNotFound(err) {
						continue
					}
					return nil, err
				}
			}
			entries = append(entries, kvEntry{key: key, encoded: name, value: val})
		}
	}

	// Add buffered sets not in OPFS.
	if t.write {
		for encoded, val := range t.sets {
			if _, ok := seen[encoded]; ok {
				continue
			}
			key, err := decodeKey(encoded)
			if err != nil {
				continue
			}
			if len(prefix) > 0 && !bytes.HasPrefix(key, prefix) {
				continue
			}
			v := val
			if !loadValues {
				v = nil
			}
			entries = append(entries, kvEntry{key: key, encoded: encoded, value: v})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i].key, entries[j].key) < 0
	})
	return entries, nil
}

// Commit commits the transaction to storage.
func (t *Tx) Commit(ctx context.Context) error {
	if t.discarded.Swap(true) {
		return kvtx.ErrDiscarded
	}
	defer t.release()

	if !t.write {
		return nil
	}

	if len(t.sets) == 0 && len(t.deletes) == 0 {
		return nil
	}

	// Write .pending marker.
	if err := t.writeFile(t.store.root, pendingMarker, []byte("1")); err != nil {
		return errors.Wrap(err, "write pending marker")
	}

	// Apply sets.
	for encoded, val := range t.sets {
		shard := shardPrefix(encoded)
		shardDir, err := t.getShardDir(shard, true)
		if err != nil {
			return errors.Wrap(err, "create shard dir")
		}
		if err := t.writeFile(shardDir, encoded, val); err != nil {
			return errors.Wrap(err, "write entry")
		}
	}

	// Apply deletes.
	for encoded := range t.deletes {
		shard := shardPrefix(encoded)
		shardDir, err := t.getShardDir(shard, false)
		if err != nil {
			if opfs.IsNotFound(err) {
				continue
			}
			return errors.Wrap(err, "get shard dir for delete")
		}
		err = opfs.DeleteFile(shardDir, encoded)
		if err != nil && !opfs.IsNotFound(err) {
			return errors.Wrap(err, "delete entry")
		}
	}

	// Remove .pending marker.
	err := opfs.DeleteFile(t.store.root, pendingMarker)
	if err != nil && !opfs.IsNotFound(err) {
		return errors.Wrap(err, "remove pending marker")
	}
	return nil
}

// Discard cancels the transaction.
func (t *Tx) Discard() {
	if t.discarded.Swap(true) {
		return
	}
	t.release()
}

// cleanupPending detects and cleans up a .pending marker from a crashed
// write transaction. Called at the start of each new write transaction.
//
// Crash semantics: OPFS file writes are atomic per file (createWritable
// replaces the entire file on close, sync access handles flush atomically).
// A crash mid-commit leaves some keys updated and others not, but no
// individual key value is corrupted. This is acceptable because:
//
//   - Block store keys are content-addressed and idempotent.
//   - Object store keys are overwritten completely on the next commit.
//   - GC ref graph edges use ignore_duplicate/ignore_missing, so partial
//     ref writes are safe (missing refs may cause premature collection
//     of unreferenced blocks, but this is bounded by the GC grace period).
//
// Read transactions do NOT check or clean the marker. They may observe
// partial state from a crashed write until the next write transaction
// runs cleanup.
func (t *Tx) cleanupPending() error {
	exists, err := opfs.FileExists(t.store.root, pendingMarker)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	err = opfs.DeleteFile(t.store.root, pendingMarker)
	if err != nil && !opfs.IsNotFound(err) {
		return err
	}
	return nil
}

// _ is a type assertion.
var _ kvtx.Tx = ((*Tx)(nil))
