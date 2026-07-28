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
// Read transactions delegate each operation to the shard.
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
	if write {
		return &metaWriteTx{
			shard:   s.shard,
			read:    metaReadTx{shard: s.shard},
			pending: make([]mutation, 0, 8),
		}, nil
	}
	return &metaReadTx{shard: s.shard}, nil
}

// metaReadTx is a read-only transaction over the committed MetaShard.
//
// Each operation opens its own committed snapshot under the shared metadata
// lock and drops both before returning. A snapshot cannot outlive its operation
// because the page store recycles freed pages on commit, and the only
// cross-agent barrier against that is the lock; a transaction that held the
// lock for its own lifetime would stall every writer in every tab until the
// caller discarded it.
//
// The transaction still owes its caller one consistent view. It pins the
// generation its first operation read and fails any later operation that would
// serve a different one, so a caller reading several keys either sees them as
// one generation held them or gets ErrGenerationChanged. Serving both would let
// it assemble metadata that never existed.
type metaReadTx struct {
	shard *MetaShard
	// generation is the commit generation this transaction is pinned to, valid
	// once pinned is set by the first operation to complete.
	generation uint64
	pinned     bool
}

// pin binds the transaction to the generation that served an operation, or
// reports that the generation moved underneath it.
func (t *metaReadTx) pin(generation uint64) error {
	if !t.pinned {
		t.generation = generation
		t.pinned = true
		return nil
	}
	if t.generation != generation {
		return errors.Wrapf(
			ErrGenerationChanged,
			"transaction pinned to generation %d, read generation %d",
			t.generation,
			generation,
		)
	}
	return nil
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
	val, found, generation, err := t.shard.getAt(key)
	if err != nil {
		return nil, false, err
	}
	if err := t.pin(generation); err != nil {
		return nil, false, err
	}
	return val, found, nil
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

// Commit is a no-op: read transactions hold nothing between operations.
func (t *metaReadTx) Commit(ctx context.Context) error {
	return nil
}

// Discard is a no-op: read transactions hold nothing between operations.
func (t *metaReadTx) Discard() {}

func (t *metaReadTx) collectPrefix(prefix []byte) ([]metaEntry, error) {
	entries, generation, err := t.shard.collectPrefixAt(prefix)
	if err != nil {
		return nil, err
	}
	if err := t.pin(generation); err != nil {
		return nil, err
	}
	return entries, nil
}

// mutation is a buffered Set or Delete operation.
type mutation struct {
	key   []byte
	value []byte // nil means delete
}

// metaWriteTx buffers mutations and commits via MetaShard.WriteTx.
type metaWriteTx struct {
	shard *MetaShard
	// read serves keys with no buffered mutation.
	read metaReadTx

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
	return t.read.Get(ctx, key)
}

// Exists checks pending mutations then the tree.
func (t *metaWriteTx) Exists(ctx context.Context, key []byte) (bool, error) {
	for i := len(t.pending) - 1; i >= 0; i-- {
		m := &t.pending[i]
		if bytes.Equal(m.key, key) {
			return m.value != nil, nil
		}
	}
	return t.read.Exists(ctx, key)
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
		return nil
	}
	t.committed = true

	if len(t.pending) == 0 {
		return nil
	}

	err := t.shard.WriteTx(func(tree *pagestore.Tree) error {
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
	return err
}

// Discard discards pending mutations.
func (t *metaWriteTx) Discard() {
	t.pending = nil
}

func (t *metaWriteTx) Size(ctx context.Context) (uint64, error) {
	return t.read.Size(ctx)
}

func (t *metaWriteTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	return t.read.ScanPrefix(ctx, prefix, cb)
}

func (t *metaWriteTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.read.ScanPrefixKeys(ctx, prefix, cb)
}

func (t *metaWriteTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return t.read.Iterate(ctx, prefix, sort, reverse)
}

// _ is a type assertion.
var _ kvtx.Store = (*MetaStore)(nil)
