//go:build js

package metashard

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
	"github.com/aperturerobotics/hydra/volume/js/opfs/pagestore"
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
	if write {
		return &metaWriteTx{
			metaReadTx: metaReadTx{shard: s.shard},
			pending:    make([]mutation, 0, 8),
		}, nil
	}
	return &metaReadTx{shard: s.shard}, nil
}

// metaReadTx is a read-only transaction delegating to MetaShard.
type metaReadTx struct {
	shard *MetaShard
}

// Size returns the number of keys. Scans the entire tree.
func (t *metaReadTx) Size(ctx context.Context) (uint64, error) {
	var count uint64
	err := t.shard.ScanPrefix(nil, func(_, _ []byte) bool {
		count++
		return true
	})
	return count, err
}

// Get looks up a key.
func (t *metaReadTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	return t.shard.Get(key)
}

// Exists checks if a key exists.
func (t *metaReadTx) Exists(ctx context.Context, key []byte) (bool, error) {
	_, found, err := t.shard.Get(key)
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
	var cbErr error
	err := t.shard.ScanPrefix(prefix, func(key, value []byte) bool {
		if err := cb(key, value); err != nil {
			cbErr = err
			return false
		}
		return true
	})
	if cbErr != nil {
		return cbErr
	}
	return err
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (t *metaReadTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	var cbErr error
	err := t.shard.ScanPrefix(prefix, func(key, _ []byte) bool {
		if err := cb(key); err != nil {
			cbErr = err
			return false
		}
		return true
	})
	if cbErr != nil {
		return cbErr
	}
	return err
}

// Iterate returns a sorted iterator for keys with the given prefix.
func (t *metaReadTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return kvtx_iterator.NewIterator(ctx, t, prefix, sort, reverse)
}

// Commit is a no-op for read transactions.
func (t *metaReadTx) Commit(ctx context.Context) error {
	return nil
}

// Discard is a no-op for read transactions.
func (t *metaReadTx) Discard() {
}

// mutation is a buffered Set or Delete operation.
type mutation struct {
	key   []byte
	value []byte // nil means delete
}

// metaWriteTx buffers mutations and commits via MetaShard.WriteTx.
type metaWriteTx struct {
	metaReadTx
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
	return t.shard.Get(key)
}

// Exists checks pending mutations then the tree.
func (t *metaWriteTx) Exists(ctx context.Context, key []byte) (bool, error) {
	for i := len(t.pending) - 1; i >= 0; i-- {
		m := &t.pending[i]
		if bytes.Equal(m.key, key) {
			return m.value != nil, nil
		}
	}
	_, found, err := t.shard.Get(key)
	return found, err
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
}

// _ is a type assertion.
var _ kvtx.Store = (*MetaStore)(nil)
