//go:build js

package metashard

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_iterator "github.com/s4wave/spacewave/db/kvtx/iterator"
	"github.com/s4wave/spacewave/db/volume/js/opfs/pagestore"
)

// MetaStore wraps a MetaShard as a kvtx.Store.
// Read transactions delegate directly to the B+tree.
// Write transactions buffer mutations and commit via WriteTx.
type MetaStore struct {
	shard *MetaShard
}

// NewMetaStore creates a kvtx.Store backed by the meta shard.
func NewMetaStore(shard *MetaShard) *MetaStore {
	return &MetaStore{shard: shard}
}

// Execute is a no-op for the meta store.
func (s *MetaStore) Execute(ctx context.Context) error {
	return nil
}

// NewTransaction returns a new transaction against the store.
func (s *MetaStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	release, err := s.shard.acquireStateLock(false)
	if err != nil {
		return nil, errors.Wrap(err, "acquire meta read lock")
	}
	tree, generation := s.shard.OpenCommittedTree()
	readTx := metaReadTx{
		shard:      s.shard,
		tree:       tree,
		generation: generation,
		release:    release,
	}
	if write {
		return &metaWriteTx{
			metaReadTx: readTx,
			shard:      s.shard,
			pending:    make([]mutation, 0, 8),
		}, nil
	}
	return &readTx, nil
}

// metaReadTx is a read-only transaction delegating to MetaShard.
type metaReadTx struct {
	shard      *MetaShard
	tree       *pagestore.Tree
	generation uint64
	recovered  bool
	release    func()
	released   bool
}

type metaEntry struct {
	key   []byte
	value []byte
}

// Size returns the number of keys. Scans the entire tree.
func (t *metaReadTx) Size(ctx context.Context) (uint64, error) {
	entries, err := t.collectPrefix(nil)
	return uint64(len(entries)), err
}

// Get looks up a key.
func (t *metaReadTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	val, found, err := t.tree.Get(key)
	if err == nil {
		return val, found, nil
	}
	if err := t.recoverCorruptRead(err); err != nil {
		return nil, false, err
	}
	return t.tree.Get(key)
}

// Exists checks if a key exists.
func (t *metaReadTx) Exists(ctx context.Context, key []byte) (bool, error) {
	_, found, err := t.Get(ctx, key)
	return found, err
}

// Set is not supported on read transactions.
func (t *metaReadTx) Set(ctx context.Context, key, value []byte) error {
	return ErrReadOnly
}

// Delete is not supported on read transactions.
func (t *metaReadTx) Delete(ctx context.Context, key []byte) error {
	return ErrReadOnly
}

// ScanPrefix iterates over entries matching the prefix.
func (t *metaReadTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	entries, err := t.collectPrefix(prefix)
	if err != nil {
		return err
	}
	for i := range entries {
		entry := &entries[i]
		if err := cb(entry.key, entry.value); err != nil {
			return err
		}
	}
	return nil
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (t *metaReadTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	entries, err := t.collectPrefix(prefix)
	if err != nil {
		return err
	}
	for i := range entries {
		if err := cb(entries[i].key); err != nil {
			return err
		}
	}
	return nil
}

// Iterate returns a sorted iterator for keys with the given prefix.
func (t *metaReadTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return kvtx_iterator.NewIterator(ctx, t, prefix, sort, reverse)
}

// Commit is a no-op for read transactions.
func (t *metaReadTx) Commit(ctx context.Context) error {
	t.releaseLock()
	return nil
}

// Discard is a no-op for read transactions.
func (t *metaReadTx) Discard() {
	t.releaseLock()
}

func (t *metaReadTx) collectPrefix(prefix []byte) ([]metaEntry, error) {
	entries, err := scanPrefixEntries(t.tree, prefix)
	if err == nil {
		return entries, nil
	}
	if err := t.recoverCorruptRead(err); err != nil {
		return nil, err
	}

	return scanPrefixEntries(t.tree, prefix)
}

func (t *metaReadTx) recoverCorruptRead(err error) error {
	if !IsCorruptError(err) {
		return err
	}
	if t.recovered {
		return err
	}
	if t.shard == nil {
		return err
	}
	hadLock := t.release != nil && !t.released
	if hadLock {
		t.releaseLock()
	}
	if err := t.shard.recoverCorruptState(); err != nil {
		return errors.Wrap(err, "recover corrupt meta shard")
	}
	if hadLock {
		release, err := t.shard.acquireStateLock(false)
		if err != nil {
			return errors.Wrap(err, "reacquire meta read lock")
		}
		t.release = release
		t.released = false
	}
	t.tree, t.generation = t.shard.OpenCommittedTree()
	t.recovered = true
	return nil
}

func (t *metaReadTx) releaseLock() {
	if t.release == nil || t.released {
		return
	}
	t.release()
	t.released = true
}

// mutation is a buffered Set or Delete operation.
type mutation struct {
	key   []byte
	value []byte // nil means delete
}

// metaWriteTx buffers mutations and commits via MetaShard.WriteTx.
type metaWriteTx struct {
	metaReadTx
	shard *MetaShard

	pending   []mutation
	committed bool
}

// Get checks pending mutations first, then falls through to the tree.
func (t *metaWriteTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	// Check pending mutations in reverse order (last write wins).
	for i := len(t.pending) - 1; i >= 0; i-- {
		m := &t.pending[i]
		if bytes.Equal(m.key, key) {
			if m.value == nil {
				return nil, false, nil // deleted
			}
			return m.value, true, nil
		}
	}
	return t.metaReadTx.Get(ctx, key)
}

// Exists checks pending mutations then the tree.
func (t *metaWriteTx) Exists(ctx context.Context, key []byte) (bool, error) {
	for i := len(t.pending) - 1; i >= 0; i-- {
		m := &t.pending[i]
		if bytes.Equal(m.key, key) {
			return m.value != nil, nil
		}
	}
	return t.metaReadTx.Exists(ctx, key)
}

// Set buffers a set operation.
func (t *metaWriteTx) Set(ctx context.Context, key, value []byte) error {
	t.pending = append(t.pending, mutation{
		key:   bytes.Clone(key),
		value: bytes.Clone(value),
	})
	return nil
}

// Delete buffers a delete operation.
func (t *metaWriteTx) Delete(ctx context.Context, key []byte) error {
	t.pending = append(t.pending, mutation{
		key:   bytes.Clone(key),
		value: nil,
	})
	return nil
}

// Commit applies all buffered mutations atomically via WriteTx.
func (t *metaWriteTx) Commit(ctx context.Context) error {
	if t.committed {
		t.releaseLock()
		return nil
	}
	t.committed = true

	if len(t.pending) == 0 {
		t.releaseLock()
		return nil
	}

	t.releaseLock()
	return t.shard.WriteTx(func(tree *pagestore.Tree) error {
		for i := range t.pending {
			m := &t.pending[i]
			if m.value == nil {
				if _, err := tree.Delete(m.key); err != nil {
					return err
				}
			} else {
				if err := tree.Put(m.key, m.value); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Discard discards pending mutations.
func (t *metaWriteTx) Discard() {
	t.pending = nil
	t.releaseLock()
}

// _ is a type assertion.
var _ kvtx.Store = (*MetaStore)(nil)
