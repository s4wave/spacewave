package s4wave_kv_world

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

type worldCursor = *bucket_lookup.Cursor

// WorldBackedStore wraps a KVTX block store and commits root updates through world ops.
type WorldBackedStore struct {
	inner *kvtx_block.Store
	ws    world.WorldState
	key   string
	root  *bucket_lookup.Cursor

	mtx         sync.Mutex
	pendingRoot *bucket.ObjectRef
}

// NewWorldBackedStore opens a KVTX store against a world object's current root.
func NewWorldBackedStore(
	ctx context.Context,
	le *logrus.Entry,
	root *bucket_lookup.Cursor,
	ws world.WorldState,
	objectKey string,
) (*WorldBackedStore, error) {
	if ws == nil {
		return nil, objecttype.ErrWorldStateRequired
	}
	if root == nil {
		return nil, errors.New("kv/store: root cursor is required")
	}
	if objectKey == "" {
		return nil, world.ErrEmptyObjectKey
	}

	st := &WorldBackedStore{
		ws:   ws,
		key:  objectKey,
		root: root,
	}
	inner, err := kvtx_block.NewStore(ctx, le, root, st.captureCommittedRoot)
	if err != nil {
		root.Release()
		return nil, err
	}
	st.inner = inner
	return st, nil
}

// Close releases the root cursor owned by the store.
func (s *WorldBackedStore) Close() {
	if s == nil || s.root == nil {
		return
	}
	s.root.Release()
	s.root = nil
}

// NewTransaction returns a KVTX transaction.
func (s *WorldBackedStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.inner.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &worldBackedTx{
		store: s,
		inner: tx,
	}, nil
}

func (s *WorldBackedStore) captureCommittedRoot(root *bucket.ObjectRef) error {
	if root == nil || root.GetEmpty() {
		return errors.New("kv/store: committed root is empty")
	}
	s.mtx.Lock()
	s.pendingRoot = root.Clone()
	s.mtx.Unlock()
	return nil
}

func (s *WorldBackedStore) clearPendingRoot() {
	s.mtx.Lock()
	s.pendingRoot = nil
	s.mtx.Unlock()
}

func (s *WorldBackedStore) takePendingRoot() *bucket.ObjectRef {
	s.mtx.Lock()
	root := s.pendingRoot
	s.pendingRoot = nil
	s.mtx.Unlock()
	return root
}

type worldBackedTx struct {
	store *WorldBackedStore
	inner kvtx.Tx
}

// Commit commits KVTX data, then advances the world object root outside KVTX locks.
func (t *worldBackedTx) Commit(ctx context.Context) error {
	t.store.clearPendingRoot()
	if err := t.inner.Commit(ctx); err != nil {
		t.store.clearPendingRoot()
		return err
	}

	root := t.store.takePendingRoot()
	if root == nil {
		return &CommitPersistedError{Err: errors.New("kv/store: committed root was not captured")}
	}
	_, _, err := t.store.ws.ApplyWorldOp(ctx, NewKvSetRootOp(t.store.key, root), peer.ID(""))
	if err != nil {
		return &CommitPersistedError{Err: err}
	}
	return nil
}

// Discard discards the transaction.
func (t *worldBackedTx) Discard() {
	t.inner.Discard()
}

// Size returns the number of keys in the transaction.
func (t *worldBackedTx) Size(ctx context.Context) (uint64, error) {
	return t.inner.Size(ctx)
}

// Get returns the value for a key.
func (t *worldBackedTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	return t.inner.Get(ctx, key)
}

// Exists checks whether a key exists.
func (t *worldBackedTx) Exists(ctx context.Context, key []byte) (bool, error) {
	return t.inner.Exists(ctx, key)
}

// Set sets a key in the transaction.
func (t *worldBackedTx) Set(ctx context.Context, key, value []byte) error {
	return t.inner.Set(ctx, key, value)
}

// Delete deletes a key in the transaction.
func (t *worldBackedTx) Delete(ctx context.Context, key []byte) error {
	return t.inner.Delete(ctx, key)
}

// ScanPrefix scans key-value pairs by prefix.
func (t *worldBackedTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	return t.inner.ScanPrefix(ctx, prefix, cb)
}

// ScanPrefixKeys scans keys by prefix.
func (t *worldBackedTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.inner.ScanPrefixKeys(ctx, prefix, cb)
}

// Iterate returns an iterator over the transaction.
func (t *worldBackedTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return t.inner.Iterate(ctx, prefix, sort, reverse)
}

var _ kvtx.Store = ((*WorldBackedStore)(nil))
var _ kvtx.Tx = ((*worldBackedTx)(nil))
