package s4wave_kv_world

import (
	"bytes"
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// WorldBackedStore wraps a KVTX block store and commits root updates through world ops.
type WorldBackedStore struct {
	inner *kvtx_block.Store
	ws    world.WorldState
	key   string
	root  *world.OwnedLookupCursor

	mtx      sync.Mutex
	writeMtx sync.Mutex
	tx       *worldBackedTx
}

// NewWorldBackedStore opens a KVTX store against a world object's current root.
func NewWorldBackedStore(
	ctx context.Context,
	le *logrus.Entry,
	root *world.OwnedLookupCursor,
	ws world.WorldState,
	objectKey string,
) (*WorldBackedStore, error) {
	if ws == nil {
		if root != nil {
			root.Release()
		}
		return nil, objecttype.ErrWorldStateRequired
	}
	if root == nil || root.Cursor() == nil {
		if root != nil {
			root.Release()
		}
		return nil, errors.New("kv/store: root cursor is required")
	}
	if objectKey == "" {
		root.Release()
		return nil, world.ErrEmptyObjectKey
	}

	st := &WorldBackedStore{
		ws:   ws,
		key:  objectKey,
		root: root,
	}
	inner, err := kvtx_block.NewStore(ctx, le, root.Cursor(), st.captureCommittedRoot)
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
}

// GetRootRef returns the current KV root reference.
func (s *WorldBackedStore) GetRootRef() *bucket.ObjectRef {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.GetRootRef()
}

// WatchPrefix streams current key/value snapshots for a prefix after world commits.
func (s *WorldBackedStore) WatchPrefix(ctx context.Context, prefix []byte, cb func(entries []kvtx.WatchEntry) error) error {
	if cb == nil {
		return nil
	}
	var prev []kvtx.WatchEntry
	var havePrev bool
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		obj, err := world.MustGetObject(ctx, s.ws, s.key)
		if err != nil {
			return err
		}
		_, rev, err := obj.GetRootRef(ctx)
		if err != nil {
			return err
		}
		entries, err := s.scanWatchPrefix(ctx, prefix)
		if err != nil {
			return err
		}
		if !havePrev || !kvWatchEntriesEqual(prev, entries) {
			if err := cb(entries); err != nil {
				return err
			}
			prev = entries
			havePrev = true
		}
		if _, err := obj.WaitRev(ctx, rev+1, false); err != nil {
			return err
		}
	}
}

func (s *WorldBackedStore) scanWatchPrefix(ctx context.Context, prefix []byte) ([]kvtx.WatchEntry, error) {
	s.writeMtx.Lock()
	tx, err := func() (kvtx.Tx, error) {
		defer s.writeMtx.Unlock()
		if err := s.refreshInnerRoot(ctx); err != nil {
			return nil, err
		}
		return s.inner.NewTransaction(ctx, false)
	}()
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	var entries []kvtx.WatchEntry
	err = tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		entries = append(entries, kvtx.WatchEntry{
			Key:   bytes.Clone(key),
			Value: bytes.Clone(value),
		})
		return nil
	})
	return entries, err
}

func kvWatchEntriesEqual(a, b []kvtx.WatchEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i].Key, b[i].Key) || !bytes.Equal(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

// NewTransaction returns a KVTX transaction.
func (s *WorldBackedStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write {
		s.writeMtx.Lock()
	}
	var baseRoot *bucket.ObjectRef
	if write {
		baseRoot = s.inner.GetRootRef()
	}
	tx, err := s.inner.NewTransaction(ctx, write)
	if err != nil || !write {
		if write {
			s.writeMtx.Unlock()
		}
		return tx, err
	}
	wtx := &worldBackedTx{
		store:    s,
		inner:    tx,
		baseRoot: baseRoot,
	}
	s.mtx.Lock()
	s.tx = wtx
	s.mtx.Unlock()
	return wtx, nil
}

func (s *WorldBackedStore) captureCommittedRoot(root *bucket.ObjectRef) error {
	if root == nil || root.GetEmpty() {
		return errors.New("kv/store: committed root is empty")
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.tx == nil {
		return errors.New("kv/store: committed root captured without active transaction")
	}
	s.tx.committedRoot = root.Clone()
	return nil
}

func (s *WorldBackedStore) clearActiveTx(tx *worldBackedTx) {
	s.mtx.Lock()
	if s.tx == tx {
		s.tx = nil
	}
	s.mtx.Unlock()
}

func (s *WorldBackedStore) refreshInnerRoot(ctx context.Context) error {
	obj, err := world.MustGetObject(ctx, s.ws, s.key)
	if err != nil {
		return err
	}
	root, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return err
	}
	return s.inner.SetRootRef(ctx, root)
}

type worldBackedTx struct {
	store         *WorldBackedStore
	inner         kvtx.Tx
	baseRoot      *bucket.ObjectRef
	committedRoot *bucket.ObjectRef
	mutations     []*KvMutation
	releaseOnce   sync.Once
}

// Commit commits KVTX data, then advances the world object root outside KVTX locks.
func (t *worldBackedTx) Commit(ctx context.Context) error {
	defer t.releaseWrite()
	t.committedRoot = nil
	if err := t.inner.Commit(ctx); err != nil {
		return err
	}

	root := t.committedRoot
	if root == nil {
		return &CommitPersistedError{Err: errors.New("kv/store: committed root was not captured")}
	}
	_, _, err := t.store.ws.ApplyWorldOp(ctx, NewKvSetRootOp(t.store.key, t.baseRoot, root, t.mutations), peer.ID(""))
	if err != nil {
		return &CommitPersistedError{Err: err}
	}
	if err := t.store.refreshInnerRoot(ctx); err != nil {
		return &CommitPersistedError{Err: err}
	}
	return nil
}

// Discard discards the transaction.
func (t *worldBackedTx) Discard() {
	t.releaseWrite()
	t.inner.Discard()
}

func (t *worldBackedTx) releaseWrite() {
	t.releaseOnce.Do(func() {
		t.store.clearActiveTx(t)
		t.store.writeMtx.Unlock()
	})
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
	if err := t.inner.Set(ctx, key, value); err != nil {
		return err
	}
	t.mutations = append(t.mutations, &KvMutation{
		Kind:  KvMutationKind_KV_MUTATION_KIND_SET,
		Key:   bytes.Clone(key),
		Value: bytes.Clone(value),
	})
	return nil
}

// Delete deletes a key in the transaction.
func (t *worldBackedTx) Delete(ctx context.Context, key []byte) error {
	if err := t.inner.Delete(ctx, key); err != nil {
		return err
	}
	t.mutations = append(t.mutations, &KvMutation{
		Kind: KvMutationKind_KV_MUTATION_KIND_DELETE,
		Key:  bytes.Clone(key),
	})
	return nil
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
