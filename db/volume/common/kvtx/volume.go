package kvtx

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	"github.com/s4wave/spacewave/db/kvtx"
	hstore "github.com/s4wave/spacewave/db/store"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
)

// StatsFn returns storage usage statistics for a volume.
type StatsFn func(ctx context.Context) (*volume.StorageStats, error)

// Volume implements a key-value volume.
type Volume struct {
	// volumeID is the volume id
	volumeID string
	// Store is the hydra store.
	hstore.Store
	// Coordinator coordinates direct multi-writer ObjectStore access.
	coord.Coordinator
	// Peer indicates the volume has a peer identity.
	peer.Peer
	// kvtxStore is the underlying kvtx store
	kvtxStore kvtx.Store
	// kvKey is the underlying kvkey
	kvKey *store_kvkey.KVKey
	// refGraph is the volume's GC reference graph.
	refGraph block_gc.RefGraphOps
	// walAppender is the optional WAL appender for deferred GC updates.
	walAppender block_gc.WALAppender
	// gcManagerHooks are the optional WAL-backed GC manager hooks.
	gcManagerHooks *block_gc.ManagerHooks
	// statsFn returns storage stats, may be nil.
	statsFn StatsFn
	// statsBcast wakes watchers when storage stats may have changed.
	statsBcast broadcast.Broadcast
	// closeFn is the close func, may be nil
	closeFn func() error
	// deleteFn removes the backing store after Close, may be nil.
	deleteFn func() error
	// publications owns bounded grouped writes in this volume's raw transaction domain.
	publications *publicationWriter
	// ordered flushes direct atomic writes committed with write ordering, may
	// be nil.
	ordered kvtx.OrderedCommitStore
	// atomicHashGet records whether hash verification reads are enabled for
	// atomic publication block construction.
	atomicHashGet bool
	// closeBcast guards the closing, closed, and closeErr state below.
	closeBcast broadcast.Broadcast
	// closing records that Close has started and direct writes must stop.
	closing bool
	// closed records that the close sequence finished.
	closed bool
	// directMu joins synchronous preparation and GC transactions before closing.
	directMu     sync.RWMutex
	directClosed bool
	// closeErr stores the error from Close.
	closeErr error
	// rootPinMu guards reader pin counts and their volume lease. Direct
	// transactions take it to carry released pins, so it is never held while
	// opening a volume transaction.
	rootPinMu      csync.Mutex
	rootPinOwner   string
	rootPinLease   coord.WriteLease
	rootPins       map[string]*rootPin
	rootPinsClosed bool
	// proofMu guards the pending root proofs and the sweep count. It is never
	// held across a volume transaction.
	proofMu sync.Mutex
	// rootProofs are the completion proofs no commit has carried yet, keyed by
	// root node.
	rootProofs map[string]*block.BlockRef
	// sweeps counts the sweep transactions that may have removed nodes.
	sweeps uint64
}

// KvtxVolume is an interface for a volume with a kvtx store.
type KvtxVolume interface {
	// KvtxVolume extends Volume
	volume.Volume

	// GetKvtxStore returns the underlying kvtx store.
	GetKvtxStore() kvtx.Store
	// GetKvKey returns the instance of KvKey used to build keys.
	GetKvKey() *store_kvkey.KVKey
}

// NewVolume builds a new key-value volume.
//
// store /may/ optionally also be a store_kvtx.Store.
func NewVolume(
	ctx context.Context,
	storeID string,
	kvkey *store_kvkey.KVKey,
	store kvtx.Store,
	conf *store_kvtx.Config,
	noGenerateKey,
	noWriteKey bool,
	statsFn StatsFn,
	closeFn func() error,
	deleteFn ...func() error,
) (*Volume, error) {
	// Build the Volume around its key-value store and cleanup callbacks.
	v := &Volume{
		Store:     store_kvtx.NewKVTx(kvkey, store, conf),
		kvtxStore: store,
		kvKey:     kvkey,
		statsFn:   statsFn,
		closeFn:   closeFn,
	}
	if len(deleteFn) != 0 {
		v.deleteFn = deleteFn[0]
	}

	// Load or claim the Volume identity and initialize its storage.
	v, err := initVolume(ctx, v, storeID, store, noGenerateKey, noWriteKey)
	if err != nil {
		return nil, err
	}

	// Coordinate direct writers using the initialized Volume identity.
	v.Coordinator = coord_inmem.ForVolume(v.GetID())

	// Route atomic publications through transaction-local reference graphs.
	if atomicStore, ok := store.(kvtx.AtomicCommitStore); ok && atomicStore.SupportsAtomicCommit() {
		v.atomicHashGet = !conf.GetDisableHashGet()
		// Long-lived Cayley caches cannot be shared with transaction-local writes.
		if v.refGraph != nil {
			_ = v.refGraph.Close()
		}
		v.refGraph = &transactionRefGraph{volume: v}
		v.publications = newPublicationWriter(v)
		if orderedStore, ok := store.(kvtx.OrderedCommitStore); ok {
			v.ordered = orderedStore
		}
	}

	return v, nil
}

// NewVolumeWithBlockStore builds a key/value volume with a custom block store.
//
// blk is used for block operations instead of creating a KVTxBlock from the
// kvtx store. This supports per-file locking block stores (e.g. OPFS).
func NewVolumeWithBlockStore(
	ctx context.Context,
	storeID string,
	kvkey *store_kvkey.KVKey,
	store kvtx.Store,
	blk block.StoreOps,
	conf *store_kvtx.Config,
	noGenerateKey,
	noWriteKey bool,
	statsFn StatsFn,
	closeFn func() error,
	deleteFn ...func() error,
) (*Volume, error) {
	// Build the Volume around its key-value store and cleanup callbacks.
	v := &Volume{
		Store:     store_kvtx.NewKVTxWithBlockStore(kvkey, store, blk, conf),
		kvtxStore: store,
		kvKey:     kvkey,
		statsFn:   statsFn,
		closeFn:   closeFn,
	}
	if len(deleteFn) != 0 {
		v.deleteFn = deleteFn[0]
	}

	// Load or claim the Volume identity and initialize its storage.
	v, err := initVolume(ctx, v, storeID, store, noGenerateKey, noWriteKey)
	if err != nil {
		return nil, err
	}

	// Coordinate direct writers using the initialized Volume identity.
	v.Coordinator = coord_inmem.ForVolume(v.GetID())

	return v, nil
}

// NewVolumeWithBlockStoreAndGC builds a key/value volume with a custom block
// store and a pre-built GC reference graph. The Cayley RefGraph is not created.
func NewVolumeWithBlockStoreAndGC(
	ctx context.Context,
	storeID string,
	kvkey *store_kvkey.KVKey,
	store kvtx.Store,
	blk block.StoreOps,
	rg block_gc.RefGraphOps,
	conf *store_kvtx.Config,
	noGenerateKey,
	noWriteKey bool,
	statsFn StatsFn,
	closeFn func() error,
	deleteFn ...func() error,
) (*Volume, error) {
	// Build the Volume around its key-value store and cleanup callbacks.
	v := &Volume{
		Store:     store_kvtx.NewKVTxWithBlockStore(kvkey, store, blk, conf),
		kvtxStore: store,
		kvKey:     kvkey,
		refGraph:  rg,
		statsFn:   statsFn,
		closeFn:   closeFn,
	}
	if len(deleteFn) != 0 {
		v.deleteFn = deleteFn[0]
	}

	// Load or claim the Volume identity and initialize its storage.
	v, err := initVolumeSkipGC(ctx, v, storeID, noGenerateKey, noWriteKey)
	if err != nil {
		return nil, err
	}

	// Coordinate direct writers using the initialized Volume identity.
	v.Coordinator = coord_inmem.ForVolume(v.GetID())

	return v, nil
}

// initVolume performs common volume initialization: peer key generation,
// volume ID computation, and GC reference graph setup.
func initVolume(
	ctx context.Context,
	v *Volume,
	storeID string,
	store kvtx.Store,
	noGenerateKey,
	noWriteKey bool,
) (*Volume, error) {
	// Initialize the Volume identity before opening its reference graph.
	v, err := initVolumeSkipGC(ctx, v, storeID, noGenerateKey, noWriteKey)
	if err != nil {
		return nil, err
	}

	// Open the persistent reference graph for Volume garbage collection.
	rg, err := block_gc.NewRefGraph(ctx, store, volumeRefGraphPrefix())
	if err != nil {
		return nil, err
	}
	v.refGraph = rg

	return v, nil
}

// initVolumeSkipGC performs common volume initialization without creating
// a Cayley-backed GC reference graph. Used when the caller provides its
// own RefGraphOps (e.g. OPFS GCGraph).
func initVolumeSkipGC(
	ctx context.Context,
	v *Volume,
	storeID string,
	noGenerateKey,
	noWriteKey bool,
) (*Volume, error) {
	// Load the stored identity.
	peerPriv, err := v.LoadPeerPriv(ctx)
	if err != nil {
		return nil, err
	}

	// Generate an identity when none is stored, and store it unless another
	// mount of the same store committed one first.
	if peerPriv == nil {
		if noGenerateKey {
			return nil, errors.New("peer private key doesn't exist")
		}
		peerPriv, _, err = crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, err
		}
		if !noWriteKey {
			peerPriv, err = v.claimPeerPriv(ctx, peerPriv)
			if err != nil {
				return nil, err
			}
		}
	}

	// Build the peer and derive the volume id from its peer id.
	v.Peer, err = peer.NewPeer(peerPriv)
	if err != nil {
		return nil, err
	}
	v.volumeID = volume.NewVolumeID(storeID, v.Peer.GetPeerID())

	return v, nil
}

// claimPeerPriv stores priv as the volume identity unless one is already
// stored, and returns the stored identity. The check and the write share one
// write transaction, so concurrent mounts of one store agree on the identity
// even when their earlier reads saw an older revision.
func (v *Volume) claimPeerPriv(ctx context.Context, priv crypto.PrivKey) (crypto.PrivKey, error) {
	// Read the identity under the writer.
	tx, err := v.kvtxStore.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	key := v.kvKey.GetPeerPrivKey()
	data, found, err := tx.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if found && len(data) != 0 {
		return keypem.ParsePrivKeyPem(data)
	}

	// Store the new identity.
	data, err = keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		return nil, err
	}
	if err := tx.Set(ctx, key, data); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return priv, nil
}

// GetID returns the computed volume id.
func (v *Volume) GetID() string {
	return v.volumeID
}

// GetPeerID returns the volume peer ID.
func (v *Volume) GetPeerID() peer.ID {
	return v.Peer.GetPeerID()
}

// GetPeer returns the Peer object.
// If withPriv=false ensure that the Peer returned does not have the private key.
func (v *Volume) GetPeer(ctx context.Context, withPriv bool) (peer.Peer, error) {
	vp := v.Peer
	if !withPriv {
		return peer.NewPeerWithPubKey(vp.GetPubKey())
	}
	return vp, nil
}

// GetKvtxStore returns the underlying kvtx store.
func (v *Volume) GetKvtxStore() kvtx.Store {
	return v.kvtxStore
}

// GetKvKey returns the instance of KvKey used to build keys.
func (v *Volume) GetKvKey() *store_kvkey.KVKey {
	return v.kvKey
}

// GetStorageStats returns storage usage statistics for the volume.
func (v *Volume) GetStorageStats(ctx context.Context) (*volume.StorageStats, error) {
	if v.statsFn != nil {
		return v.statsFn(ctx)
	}
	return &volume.StorageStats{}, nil
}

// GetStorageStatsSnapshotWithWait returns storage usage statistics and a wait
// channel that closes when storage stats may have changed.
func (v *Volume) GetStorageStatsSnapshotWithWait(
	ctx context.Context,
) (*volume.StorageStats, <-chan struct{}, error) {
	// Subscribe to Volume statistics changes before reading the snapshot.
	var waitCh <-chan struct{}
	v.statsBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})

	// Read the Volume statistics paired with the change notification.
	stats, err := v.GetStorageStats(ctx)
	if err != nil {
		return nil, nil, err
	}
	return stats, waitCh, nil
}

func (v *Volume) broadcastStorageStatsChanged() {
	v.statsBcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		broadcastFn()
	})
}

// GetRefGraph returns the volume's GC reference graph.
func (v *Volume) GetRefGraph() block_gc.RefGraphOps {
	return v.refGraph
}

// PutBlockBatch forwards batched writes to the embedded store when supported.
func (v *Volume) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	if err := v.Store.PutBlockBatch(ctx, entries); err != nil {
		return err
	}
	if len(entries) != 0 {
		v.broadcastStorageStatsChanged()
	}
	return nil
}

// PutBlock forwards block writes to the embedded store.
func (v *Volume) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Write the block while retaining the requested Volume durability barrier.
	putOpts, syncRequested := block.PutOptsWithoutSync(opts)
	ref, exists, err := v.Store.PutBlock(ctx, data, putOpts)
	if err != nil {
		return nil, exists, err
	}

	// Wake Volume statistics watchers when the block adds stored data.
	if ref != nil && !exists {
		v.broadcastStorageStatsChanged()
	}

	// Make the Volume durable when the block write requests synchronization.
	if syncRequested {
		if _, err := v.Sync(ctx); err != nil {
			return ref, exists, err
		}
	}
	return ref, exists, nil
}

// GetBlockExistsBatch forwards batched existence probes to the embedded store when supported.
func (v *Volume) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return v.Store.GetBlockExistsBatch(ctx, refs)
}

// RmBlock forwards block deletion to the embedded store.
func (v *Volume) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if err := v.Store.RmBlock(ctx, ref); err != nil {
		return err
	}
	v.broadcastStorageStatsChanged()
	return nil
}

// Sync forwards the durability barrier to the embedded store.
func (v *Volume) Sync(ctx context.Context) (bool, error) {
	// Fence queued publications before synchronizing the Volume store.
	if v.publications != nil {
		if err := v.publications.fence(ctx); err != nil {
			return false, err
		}
	}

	// Synchronize block bytes through the embedded Volume store.
	fenced, err := v.Store.Sync(ctx)
	if err != nil {
		return false, err
	}

	// Flush ordered direct commits before waking statistics watchers.
	if v.ordered != nil {
		if err := v.ordered.Sync(ctx); err != nil {
			return false, err
		}
	}
	v.broadcastStorageStatsChanged()
	return fenced, nil
}

// OrdersWrites reports whether direct atomic writes commit with write
// ordering. Block and object store writes share one store, whose next full
// Commit makes every earlier ordered commit durable.
func (v *Volume) OrdersWrites() bool {
	return v.ordered != nil
}

// WaitDurable waits until every ordered commit completed before the call is
// durable, without forcing a flush.
func (v *Volume) WaitDurable(ctx context.Context) error {
	if v.ordered == nil {
		return nil
	}
	return v.ordered.WaitDurable(ctx)
}

// BeginDeferFlush forwards the GC defer-flush scope to the embedded store.
func (v *Volume) BeginDeferFlush() {
	block.BeginDeferFlush(v.Store)
}

// EndDeferFlush forwards closing the GC defer-flush scope to the embedded store.
func (v *Volume) EndDeferFlush(ctx context.Context) error {
	if err := block.EndDeferFlush(ctx, v.Store); err != nil {
		return err
	}
	v.broadcastStorageStatsChanged()
	return nil
}

// GetWALAppender returns the volume's WAL appender, if any.
// When non-nil, GCStoreOps should use this for FlushPending instead
// of calling ApplyRefBatch on the RefGraph directly.
func (v *Volume) GetWALAppender() block_gc.WALAppender {
	return v.walAppender
}

// SetWALAppender sets the WAL appender on the volume.
func (v *Volume) SetWALAppender(wal block_gc.WALAppender) {
	v.walAppender = wal
}

// GetGCManagerHooks returns the volume's WAL-backed GC manager hooks, if any.
func (v *Volume) GetGCManagerHooks() (block_gc.ManagerHooks, bool) {
	if v.gcManagerHooks == nil {
		return block_gc.ManagerHooks{}, false
	}
	return *v.gcManagerHooks, true
}

// SetGCManagerHooks stores the WAL-backed GC manager hooks on the volume.
func (v *Volume) SetGCManagerHooks(hooks block_gc.ManagerHooks) {
	v.gcManagerHooks = &hooks
}

// Close closes the volume, returning any errors.
// Close is idempotent: subsequent calls return the same error.
func (v *Volume) Close() error {
	// Claim the Volume close sequence or observe the existing closer.
	var waitCh <-chan struct{}
	started := false
	v.closeBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		// Leave an already closed Volume unchanged.
		if v.closed {
			return
		}

		// Subscribe to completion when another caller is closing the Volume.
		if v.closing {
			waitCh = getWaitCh()
			return
		}

		// Stop new Volume writes and claim responsibility for cleanup.
		v.closing = true
		started = true
		broadcast()
	})

	// Join the existing Volume close sequence when another caller claimed it.
	if !started {
		if waitCh != nil {
			<-waitCh
		}
		return v.closeErr
	}

	// Release root pins and join direct Volume transactions before closing storage.
	closeErr := v.closeRootPins()
	v.directMu.Lock()
	v.directClosed = true
	v.directMu.Unlock()

	// Drain Volume publications and close the reference graph and backing store.
	if v.publications != nil {
		v.publications.close()
	}
	if v.refGraph != nil {
		closeErr = errors.Join(closeErr, v.refGraph.Close())
	}
	if v.closeFn != nil {
		closeErr = errors.Join(closeErr, v.closeFn())
	}

	// Publish the Volume close result to all waiting callers.
	v.closeBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		v.closeErr = closeErr
		v.closed = true
		broadcast()
	})
	return v.closeErr
}

// Delete closes the volume and removes the backing store.
func (v *Volume) Delete() error {
	if err := v.Close(); err != nil {
		return err
	}
	if v.deleteFn != nil {
		if err := v.deleteFn(); err != nil {
			return err
		}
	}
	v.broadcastStorageStatsChanged()
	return nil
}

// _ is a type assertion
var (
	_ volume.Volume = (*Volume)(nil)
	_ KvtxVolume    = (*Volume)(nil)
)
