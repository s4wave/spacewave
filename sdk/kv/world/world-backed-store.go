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
	// inner is the KVTX block store at the object root.
	inner *kvtx_block.Store
	// ws publishes committed roots.
	ws world.WorldState
	// key is the world object key.
	key string
	// release releases the store's cursor and the stage that owns its writes.
	release func()

	// mtx guards tx.
	mtx sync.Mutex
	// writeMtx serializes write transactions.
	writeMtx sync.Mutex
	// tx is the open write transaction, if any.
	tx *worldBackedTx
}

// NewWorldBackedStore opens a KVTX store against a world object's current
// root. The store stages its writes until it publishes the root that
// references them; Close releases the stage.
func NewWorldBackedStore(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	objectKey string,
) (*WorldBackedStore, error) {
	// Require the World state, root cursor, and object key for the store.
	if ws == nil {
		return nil, objecttype.ErrWorldStateRequired
	}
	if objectKey == "" {
		return nil, world.ErrEmptyObjectKey
	}

	// Open a staged cursor at the object's current root.
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	rootRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, err
	}
	root, release, err := world.OpenStagedCursor(ctx, ws, rootRef)
	if err != nil {
		return nil, err
	}

	// Open the KVTX store on the staged root.
	st := &WorldBackedStore{
		ws:      ws,
		key:     objectKey,
		release: release,
	}
	inner, err := kvtx_block.NewStore(ctx, le, root, st.captureCommittedRoot)
	if err != nil {
		release()
		return nil, err
	}
	st.inner = inner
	return st, nil
}

// Close releases the root cursor and stage owned by the store.
func (s *WorldBackedStore) Close() {
	if s == nil || s.release == nil {
		return
	}
	s.release()
	s.release = nil
}

// WatchPrefix streams current key/value snapshots for a prefix after world commits.
func (s *WorldBackedStore) WatchPrefix(ctx context.Context, prefix []byte, cb func(entries []kvtx.WatchEntry) error) error {
	return s.WatchPrefixBounded(ctx, prefix, kvtx.WatchLimits{}, cb)
}

// WatchPrefixBounded streams current key/value snapshots for a prefix after
// world commits while each snapshot stays within limits.
// Returns ErrWatchLimit without any callback when a snapshot exceeds limits.
func (s *WorldBackedStore) WatchPrefixBounded(ctx context.Context, prefix []byte, limits kvtx.WatchLimits, cb func(entries []kvtx.WatchEntry) error) error {
	// Skip the World watch when there is no snapshot consumer.
	if cb == nil {
		return nil
	}

	// Retain the World object and the last delivered snapshot for the watch.
	var prev []kvtx.WatchEntry
	var havePrev bool
	obj, err := world.MustGetObject(ctx, s.ws, s.key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}

	// Deliver changed snapshots as the World object advances its revision.
	for {
		// Stop the World watch when its context is canceled.
		if err := ctx.Err(); err != nil {
			return err
		}

		// Read the World revision before scanning its current KV snapshot.
		_, rev, err := obj.GetRootRef(ctx)
		if err != nil {
			return err
		}

		// Scan the prefix within the requested snapshot limits.
		entries, err := s.scanWatchPrefix(ctx, prefix, limits)
		if err != nil {
			return err
		}

		// Deliver the prefix snapshot only when its entries have changed.
		if !havePrev || !kvWatchEntriesEqual(prev, entries) {
			if err := cb(entries); err != nil {
				return err
			}
			prev = entries
			havePrev = true
		}

		// Wait for the World object to publish its next revision.
		if _, err := obj.WaitRev(ctx, rev+1, false); err != nil {
			return err
		}
	}
}

// scanWatchPrefix scans one bounded snapshot in stable sorted key order.
// It checks the record count before loading another value and the key and value
// bytes before cloning the next entry, returning ErrWatchLimit with no partial
// snapshot.
func (s *WorldBackedStore) scanWatchPrefix(ctx context.Context, prefix []byte, limits kvtx.WatchLimits) ([]kvtx.WatchEntry, error) {
	// Refresh the block root and open a read transaction between writes.
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

	// Scan sorted prefix entries while accounting for snapshot limits.
	var (
		entries    []kvtx.WatchEntry
		numBytes   uint64
		numRecords uint64
	)
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		// Enforce the snapshot record limit before loading another value.
		if limits.MaxRecords != 0 && numRecords >= limits.MaxRecords {
			return nil, kvtx.ErrWatchLimit
		}

		// Load the next prefix entry and enforce the snapshot byte limit.
		key := it.Key()
		value, err := it.Value()
		if err != nil {
			return nil, err
		}
		if limits.MaxBytes != 0 && numBytes+uint64(len(key))+uint64(len(value)) > limits.MaxBytes {
			return nil, kvtx.ErrWatchLimit
		}

		// Retain an independent entry and account for its snapshot size.
		entries = append(entries, kvtx.WatchEntry{
			Key:   bytes.Clone(key),
			Value: bytes.Clone(value),
		})
		numRecords++
		numBytes += uint64(len(key)) + uint64(len(value))
	}

	// Report an iterator failure before returning the completed snapshot.
	if err := it.Err(); err != nil {
		return nil, err
	}
	return entries, nil
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
	// Serialize writers and retain the block root their mutations start from.
	if write {
		s.writeMtx.Lock()
	}
	var baseRoot *bucket.ObjectRef
	if write {
		baseRoot = s.inner.GetRootRef()
	}

	// Open the block transaction and release the writer lock on failure.
	tx, err := s.inner.NewTransaction(ctx, write)
	if err != nil || !write {
		if write {
			s.writeMtx.Unlock()
		}
		return tx, err
	}

	// Register the World transaction to capture the next committed block root.
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
	// Require a populated block root before attaching it to a transaction.
	if root == nil || root.GetEmpty() {
		return errors.New("kv/store: committed root is empty")
	}

	// Capture the committed block root under the active transaction lock.
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
	// Retain the World object while reading its current root.
	obj, err := world.MustGetObject(ctx, s.ws, s.key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}

	// Read the World root for the inner block store.
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
	// Commit the block transaction while retaining its writer lock.
	defer t.releaseWrite()
	t.committedRoot = nil
	if err := t.inner.Commit(ctx); err != nil {
		return err
	}

	// Require the block commit to have captured its persisted root.
	root := t.committedRoot
	if root == nil {
		return &CommitPersistedError{Err: errors.New("kv/store: committed root was not captured")}
	}

	// Advance the World root with the transaction mutations.
	_, _, err := t.store.ws.ApplyWorldOp(ctx, NewKvSetRootOp(t.store.key, t.baseRoot, root, t.mutations), peer.ID(""))
	if err != nil {
		return &CommitPersistedError{Err: err}
	}

	// Refresh the block store from the root accepted by the World operation.
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

var (
	_ kvtx.Store = (*WorldBackedStore)(nil)
	_ kvtx.Tx    = (*worldBackedTx)(nil)
)
