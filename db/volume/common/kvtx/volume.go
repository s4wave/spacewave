package kvtx

import (
	"context"
	"errors"
	"sync"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	hstore "github.com/s4wave/spacewave/db/store"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/volume"
)

// StatsFn returns storage usage statistics for a volume.
type StatsFn func(ctx context.Context) (*volume.StorageStats, error)

// Volume implements a key-value volume.
type Volume struct {
	// volumeID is the volume id
	volumeID string
	// Store is the hydra store.
	hstore.Store
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
	// closeFn is the close func, may be nil
	closeFn func() error
	// deleteFn removes the backing store after Close, may be nil.
	deleteFn func() error
	// closeOnce ensures Close is idempotent.
	closeOnce sync.Once
	// closeErr stores the error from Close.
	closeErr error
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

// NewVolume builds a new key/value volume.
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
	return initVolume(ctx, v, storeID, store, noGenerateKey, noWriteKey)
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
	return initVolume(ctx, v, storeID, store, noGenerateKey, noWriteKey)
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
	return initVolumeSkipGC(ctx, v, storeID, noGenerateKey, noWriteKey)
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
	v, err := initVolumeSkipGC(ctx, v, storeID, noGenerateKey, noWriteKey)
	if err != nil {
		return nil, err
	}

	rg, err := block_gc.NewRefGraph(ctx, store, []byte("gc/"))
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
	peerPriv, err := v.LoadPeerPriv(ctx)
	if err != nil {
		return nil, err
	}
	if peerPriv == nil {
		if noGenerateKey {
			return nil, errors.New("peer private key doesn't exist")
		}
	}

	// generates private key w/ default type if peerPriv is nil
	v.Peer, err = peer.NewPeer(peerPriv)
	if err != nil {
		return nil, err
	}

	npriv, err := v.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}
	if !noWriteKey && (peerPriv == nil || !npriv.Equals(peerPriv)) {
		peerPriv = npriv
		if err := v.StorePeerPriv(ctx, peerPriv); err != nil {
			return nil, err
		}
	}

	// calcuate the volume id based on the peer id
	v.volumeID = volume.NewVolumeID(storeID, v.Peer.GetPeerID())

	return v, nil
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

// GetRefGraph returns the volume's GC reference graph.
func (v *Volume) GetRefGraph() block_gc.RefGraphOps {
	return v.refGraph
}

// PutBlockBatch forwards batched writes to the embedded store when supported.
func (v *Volume) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	if batcher, ok := v.Store.(block.BatchPutStore); ok {
		return batcher.PutBlockBatch(ctx, entries)
	}
	for _, entry := range entries {
		if entry.Tombstone {
			if err := v.Store.RmBlock(ctx, entry.Ref); err != nil {
				return err
			}
			continue
		}
		if _, _, err := v.Store.PutBlock(ctx, entry.Data, &block.PutOpts{
			ForceBlockRef: entry.Ref.Clone(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// GetBlockExistsBatch forwards batched existence probes to the embedded store when supported.
func (v *Volume) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	if batcher, ok := v.Store.(block.BatchExistsStore); ok {
		return batcher.GetBlockExistsBatch(ctx, refs)
	}

	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := v.Store.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

// PutBlockBackground forwards background writes to the embedded store when supported.
func (v *Volume) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if bg, ok := v.Store.(block.BackgroundPutStore); ok {
		return bg.PutBlockBackground(ctx, data, opts)
	}
	return v.Store.PutBlock(ctx, data, opts)
}

// BeginDeferFlush forwards deferred-flush scope entry to the embedded store when supported.
func (v *Volume) BeginDeferFlush() {
	if df, ok := v.Store.(block.DeferFlushable); ok {
		df.BeginDeferFlush()
	}
}

// EndDeferFlush forwards deferred-flush scope exit to the embedded store when supported.
func (v *Volume) EndDeferFlush(ctx context.Context) error {
	if df, ok := v.Store.(block.DeferFlushable); ok {
		return df.EndDeferFlush(ctx)
	}
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
	v.closeOnce.Do(func() {
		if v.refGraph != nil {
			if err := v.refGraph.Close(); err != nil {
				v.closeErr = err
				return
			}
		}
		if v.closeFn != nil {
			v.closeErr = v.closeFn()
		}
	})
	return v.closeErr
}

// Delete closes the volume and removes the backing store.
func (v *Volume) Delete() error {
	if err := v.Close(); err != nil {
		return err
	}
	if v.deleteFn != nil {
		return v.deleteFn()
	}
	return nil
}

// _ is a type assertion
var (
	_ volume.Volume            = ((*Volume)(nil))
	_ KvtxVolume               = ((*Volume)(nil))
	_ block.BatchPutStore      = ((*Volume)(nil))
	_ block.BackgroundPutStore = ((*Volume)(nil))
	_ block.DeferFlushable     = ((*Volume)(nil))
)
