package provider_spacewave

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/core/provider/spacewave/synctelemetry"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/bucket"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_order "github.com/s4wave/spacewave/db/packfile/order"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

const (
	httpReaderAtReadAheadSize = 1 * 1024 * 1024
	forceSyncTimeout          = 30 * time.Second
)

// blockStoreBucketConfigRev 2 keeps the account-owned cache local-only.
// SessionTransport child buses provide the optional direct lookup layer.
const blockStoreBucketConfigRev = 2

// publicReadRemoteRefresher fetches the authoritative anonymous manifest.
type publicReadRemoteRefresher interface {
	// Refresh fetches the manifest and applies it.
	Refresh(context.Context) error
}

// BlockStore wraps a block store overlay with packfile-backed cloud storage.
type BlockStore struct {
	// store is the inner block store overlay.
	store block_store.Store
	// decodedBlocks is the decoded-block cache owned by the block-store lifecycle.
	decodedBlocks *block.DecodedBlockCache
	// forceSync flushes pending dirty blocks to the cloud immediately.
	forceSync func(ctx context.Context) error
	// syncer coalesces block uploads with mounted SharedObject publications.
	syncer *syncController
	// retention shares graph proofs and deletion exclusion across scoped handles.
	retention *blockPublicationRetention
	// refreshRemote pulls remote packfile metadata into the local read manifest.
	refreshRemote func(ctx context.Context) error
	// remoteSequence returns the local manifest's last-seen remote sequence.
	remoteSequence func(ctx context.Context) (uint64, error)
	// cacheStore is the account-owned local cache store used for non-dirty reads
	// and writes.
	cacheStore block.StoreOps
	// cloudStore is the account-owned Cloud read store.
	cloudStore block.StoreOps
	// files serves the blocks of linked files.
	files *block_store_filesegment.Store
}

// GetID returns the inner store id.
func (b *BlockStore) GetID() string {
	return b.store.GetID()
}

// GetStoreParts returns the overlay of the local cache and the cloud.
func (b *BlockStore) GetStoreParts() []block.StorePart {
	return []block.StorePart{{Label: "overlay", Store: b.store}}
}

// GetHashType returns the inner store hash type.
func (b *BlockStore) GetHashType() hash.HashType {
	return b.store.GetHashType()
}

// GetSupportedFeatures returns the native feature bitset for the inner store.
func (b *BlockStore) GetSupportedFeatures() block.StoreFeature {
	return b.store.GetSupportedFeatures()
}

// GetFileSegments returns the store serving the blocks of linked files.
func (b *BlockStore) GetFileSegments() *block_store_filesegment.Store {
	return b.files
}

// GetDecodedBlockCache returns the lifecycle-owned decoded-block cache.
func (b *BlockStore) GetDecodedBlockCache() *block.DecodedBlockCache {
	return b.decodedBlocks
}

// ReclaimStorage runs a storage reclaim pass through the sync controller.
// Returns zero without calling fence for a store that does not sync.
func (b *BlockStore) ReclaimStorage(ctx context.Context, fence bstore.ReclaimFence) (time.Time, error) {
	if b.syncer == nil {
		return time.Time{}, nil
	}
	return b.syncer.ReclaimStorage(ctx, fence)
}

// InvalidateDecodedBlockRef removes decoded-cache entries for ref.
func (b *BlockStore) InvalidateDecodedBlockRef(ctx context.Context, ref *block.BlockRef) {
	b.decodedBlocks.InvalidateRef(ctx, ref)
}

// BeginReadOperation opens a read scope on the inner store.
func (b *BlockStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	// Open the inner scope and wrap its store when it is not a full Store.
	store, release, err := b.store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	scopedStore, ok := store.(block_store.Store)
	if !ok {
		scopedStore = block_store.NewStore(b.store.GetID(), store)
	}

	// Share the cache, sync and retention state with the scope.
	return &BlockStore{
		store:          scopedStore,
		decodedBlocks:  b.decodedBlocks,
		forceSync:      b.forceSync,
		syncer:         b.syncer,
		retention:      b.retention,
		refreshRemote:  b.refreshRemote,
		remoteSequence: b.remoteSequence,
		files:          b.files,
	}, release, nil
}

// PutBlock forwards to the inner store.
func (b *BlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return b.store.PutBlock(ctx, data, opts)
}

// PutBlockBatch forwards batched writes to the inner store.
func (b *BlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Hold publication retention while a batch deletes any block.
	for _, entry := range entries {
		if entry != nil && entry.Tombstone {
			release, err := b.retention.invalidate(ctx)
			if err != nil {
				return nil, err
			}
			defer release()
			break
		}
	}

	// Tombstone publication and the decoded cache are separate lower stores;
	// invalidate on both sides so in-flight decoded stores cannot cross the
	// mutation boundary.
	b.invalidateBatchTombstones(ctx, entries)
	existed, err := b.store.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	b.invalidateBatchTombstones(ctx, entries)
	return existed, nil
}

// GetBlock forwards to the inner store.
func (b *BlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return b.store.GetBlock(ctx, ref)
}

// GetStoredBlock forwards to the inner store.
func (b *BlockStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return b.store.GetStoredBlock(ctx, ref)
}

// GetBlockExists forwards to the inner store.
func (b *BlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return b.store.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch forwards batched existence probes to the inner store.
func (b *BlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return b.store.GetBlockExistsBatch(ctx, refs)
}

// RmBlock forwards to the inner store.
func (b *BlockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	// Hold publication retention through the delete.
	release, err := b.retention.invalidate(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Delete publication and the decoded cache are separate lower stores;
	// invalidate on both sides so in-flight decoded stores cannot cross the
	// mutation boundary.
	b.InvalidateDecodedBlockRef(ctx, ref)
	if err := b.store.RmBlock(ctx, ref); err != nil {
		return err
	}
	b.InvalidateDecodedBlockRef(ctx, ref)
	return nil
}

// invalidateBatchTombstones evicts decoded values for deleted batch entries.
func (b *BlockStore) invalidateBatchTombstones(ctx context.Context, entries []*block.PutBatchEntry) {
	for _, entry := range entries {
		if entry != nil && entry.Tombstone {
			b.InvalidateDecodedBlockRef(ctx, entry.Ref)
		}
	}
}

// StatBlock forwards to the inner store.
func (b *BlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return b.store.StatBlock(ctx, ref)
}

// Sync forwards the durability barrier to the inner store.
func (b *BlockStore) Sync(ctx context.Context) (bool, error) {
	return b.store.Sync(ctx)
}

// ForceSync flushes any pending dirty blocks to the cloud immediately.
//
// A caller told that a sync is complete may rely on every reached block being
// on the remote; a sync that held a block back never reports complete, and
// returns an error matching block.ErrBlockCorrupt.
func (b *BlockStore) ForceSync(ctx context.Context) error {
	if b.forceSync == nil {
		return nil
	}
	flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forceSyncTimeout)
	defer cancel()
	return b.forceSync(flushCtx)
}

// RefreshRemote pulls remote packfile metadata into the local read manifest.
func (b *BlockStore) RefreshRemote(ctx context.Context) error {
	if b.refreshRemote == nil {
		return nil
	}
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), forceSyncTimeout)
	defer cancel()
	return b.refreshRemote(refreshCtx)
}

// RemoteSequence returns the last-seen remote packfile sequence.
func (b *BlockStore) RemoteSequence(ctx context.Context) (uint64, error) {
	if b.remoteSequence == nil {
		return 0, nil
	}
	return b.remoteSequence(ctx)
}

// NewBlockStoreRef builds a new BlockStoreRef for the cloud provider.
func NewBlockStoreRef(providerID, providerAccountID, bstoreID string) *bstore.BlockStoreRef {
	return &bstore.BlockStoreRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                bstoreID,
			ProviderId:        providerID,
			ProviderAccountId: providerAccountID,
		},
	}
}

// bstoreTracker tracks a BlockStore in the ProviderAccount.
type bstoreTracker struct {
	// a is the provider account.
	a *ProviderAccount
	// id is the bstore id.
	id string
	// bstoreCtr is the bstore container.
	bstoreCtr *ccontainer.CContainer[*BlockStore]
	// errCh receives access-gated errors that should not retry on a timer.
	errCh chan error
}

// buildBlockStoreTracker builds a new bstoreTracker for a bstore id.
func (a *ProviderAccount) buildBlockStoreTracker(bstoreID string) (keyed.Routine, *bstoreTracker) {
	tracker := &bstoreTracker{
		a:         a,
		id:        bstoreID,
		bstoreCtr: ccontainer.NewCContainer[*BlockStore](nil),
		errCh:     make(chan error, 1),
	}
	return tracker.executeBlockStoreTracker, tracker
}

// executeBlockStoreTracker executes the bstoreTracker for the bstore.
func (t *bstoreTracker) executeBlockStoreTracker(rctx context.Context) error {
	// Scope the mount to this run and name the volume it lives on.
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()
	le := t.a.le.WithField("bstore-id", t.id)
	le.Debug("mounting cloud bstore")
	accountID := t.a.accountID
	volID := t.a.vol.GetID()

	// Mount ObjectStore for metadata (manifest, index cache, dirty tracking).
	objStoreID := BlockStoreObjectStoreID(accountID, t.id)
	objHandle, _, objRef, err := volume.ExBuildObjectStoreAPI(ctx, t.a.p.b,
		false, objStoreID, volID, ctxCancel)
	if err != nil {
		return errors.Wrap(err, "mounting object store")
	}
	defer objRef.Release()
	objStore := objHandle.GetObjectStore()

	// Build manifest from object store.
	mfst, err := manifest.New(ctx, objStore)
	if err != nil {
		return errors.Wrap(err, "building manifest")
	}

	// Build index cache.
	idxCache := manifest.NewIndexCache(objStore)

	// Build the block store handle.
	decodedBlocks := block.NewDecodedBlockCache()
	defer decodedBlocks.Close()

	// Build lower store (read-only, packfile-backed).
	lower, publicRemote := t.buildLowerStore(ctx, idxCache, decodedBlocks)
	lower.UpdateManifest(mfst.GetEntries())
	releaseSyncTelemetry := t.a.syncTelemetry.RegisterStore(t.id, lower)
	defer releaseSyncTelemetry()
	if publicRemote != nil {
		if err := publicRemote.Refresh(ctx); err != nil {
			le.WithError(err).Warn("public-read CDN root refresh failed")
		}
		releaseCdnRootRefresh := t.registerPublicReadCdnRootRefresh(ctx, le, publicRemote)
		defer releaseCdnRootRefresh()
	}

	// Mount a Bucket for the upper block cache.
	bucketConf, err := t.buildBucketConf()
	if err != nil {
		return errors.Wrap(err, "building bucket config")
	}
	applyResult, err := bucket.ExApplyBucketConfig(ctx, t.a.p.b,
		bucket.NewApplyBucketConfigToVolume(bucketConf, volID))
	if err != nil {
		return errors.Wrap(err, "applying bucket config")
	}
	if errStr := applyResult.GetError(); errStr != "" {
		return errors.New(errStr)
	}

	// Open the bucket as the upper cache.
	bucketHandle, _, bucketHandleRef, err := bucket.ExBuildBucketAPI(ctx, t.a.p.b,
		false, bucketConf.GetId(), volID, ctxCancel)
	if err != nil {
		return errors.Wrap(err, "mounting bucket")
	}
	defer bucketHandleRef.Release()
	if !bucketHandle.GetExists() {
		return errors.New("bucket does not exist after creating it")
	}
	upper := bucketHandle.GetBucket()

	// Enable co-block writeback on the packfile store: when a block is
	// fetched from a remote packfile, neighboring blocks within the same
	// physical window are also fetched and written to upper. The bare
	// upper bucket is used (not dirtyUpper) because packfile-derived
	// blocks already live in the cloud and must not be re-pushed by sync.
	lower.SetWriteback(ctx, upper, 0)

	// The upper wrapper distinguishes existing cache hits from blocks supplied
	// by demand-driven DEX; the lower wrapper observes the single Cloud fallback.
	sourceUpper := &sourceTrackingStore{
		StoreOps:   upper,
		telemetry:  &t.a.syncTelemetry,
		bstoreID:   t.id,
		source:     SyncTelemetryBlockSourceDirect,
		upperCache: true,
	}
	sourceLower := &sourceTrackingStore{
		StoreOps:  lower,
		telemetry: &t.a.syncTelemetry,
		bstoreID:  t.id,
		source:    SyncTelemetryBlockSourceCloud,
	}

	// Snapshot under the account lock: another mounted Session can replace
	// the shared signing client while this tracker starts.
	syncConf := t.a.conf.GetSync()
	sc := &syncController{
		le:         le.WithField("component", "sync"),
		store:      objStore,
		client:     t.a.currentSessionClient(),
		resourceID: t.id,
		mfst:       mfst,
		lower:      lower,
		remote:     nil,
		upper:      upper,
		childLists: t.getChildLists(),
		conf:       syncConf,
		telemetry:  &t.a.syncTelemetry,
		gateBcast:  &t.a.accountBcast,
		skipPull:   publicRemote != nil,
	}

	// Serve linked files behind the upper cache, from an index beside the
	// sync metadata, and upload their chunks through the linked lane. A file
	// hit is the block's copy, so it fills no cache.
	if err := sc.initLanes(ctx, le, publicRemote, kvtx_prefixer.NewPrefixer(objStore, []byte("linked-files/"))); err != nil {
		return errors.Wrap(err, "opening linked file index")
	}
	files := sc.linked.files
	filedUpper := sc.wrapLinkedFiles(sourceUpper)

	// Wrap upper with dirty tracking for the sync controller, before any
	// write can reach it.
	dirtyUpper := block_store_writeback.NewMarkingStore(filedUpper, sc.MarkDirty)

	// Build the handle over the overlay of the cache and the cloud.
	localID := BlockStoreID(accountID, t.id)
	overlay := newCloudOverlay(ctx, le, sourceLower, dirtyUpper)
	bstoreHandle := &BlockStore{
		store:         block_store.NewStore(localID, overlay),
		decodedBlocks: decodedBlocks,
		cacheStore:    filedUpper,
		cloudStore:    sourceLower,
		files:         files,
		retention: &blockPublicationRetention{
			store: kvtx_prefixer.NewPrefixer(objStore, []byte("publication-retention/")),
		},
	}

	// Build and register block store controller on the bus.
	bstoreCtrl := block_store_controller.NewController(
		le,
		controller.NewInfo(ControllerID+"/bstore", Version, "cloud block store for: "+localID),
		block_store_controller.NewBlockStoreBuilder(bstoreHandle),
		[]string{localID},
		true,
		[]string{localID},
		false,
		false,
	)
	relBstoreCtrl, err := t.a.p.b.AddController(ctx, bstoreCtrl, nil)
	if err != nil {
		return errors.Wrap(err, "adding bstore controller")
	}
	defer relBstoreCtrl()

	// Wire the sync controller into the handle.
	bstoreHandle.syncer = sc
	bstoreHandle.forceSync = sc.FlushNowUnordered
	bstoreHandle.refreshRemote = sc.PullNow
	bstoreHandle.remoteSequence = sc.LastPullSequence

	// Pull when the websocket reports a new block store nonce.
	if !sc.skipPull && t.a.wsTracker != nil {
		t.a.wsTracker.RegisterBlockStoreNonceCallback(t.id, sc.TriggerRemotePull)
		defer t.a.wsTracker.UnregisterBlockStoreNonceCallback(t.id)
	}

	// Run the initial pull. If access is gated, signal the error to mount
	// callers and block to prevent keyed retry.
	if err := sc.Init(ctx); err != nil {
		if clouderror.IsAccessGated(err) {
			le.WithError(err).Warn("block store access gated, not mounting")
			select {
			case t.errCh <- err:
			default:
			}
			<-ctx.Done()
			return context.Canceled
		}
		return err
	}

	// Report the initial remote sequence and start sync ownership.
	remoteSequence, err := sc.LastPullSequence(ctx)
	if err != nil {
		return errors.Wrap(err, "reading initial cloud remote sequence")
	}
	t.a.syncTelemetry.SetCloudRemoteSequence(t.id, remoteSequence)
	syncOwner := newBstoreSyncOwner(le, sc)
	syncOwner.Start(ctx)
	defer syncOwner.Stop()

	// Publish only after initial remote state and sync ownership are ready.
	le.Debug("mounted cloud bstore")
	t.bstoreCtr.SetValue(bstoreHandle)

	// Withdraw the handle when the run ends.
	<-ctx.Done()
	t.bstoreCtr.SetValue(nil)
	return context.Canceled
}

// registerPublicReadCdnRootRefresh retains refresh jobs until the returned release.
func (t *bstoreTracker) registerPublicReadCdnRootRefresh(
	ctx context.Context,
	le *logrus.Entry,
	publicRemote publicReadRemoteRefresher,
) func() {
	cdnRootRefreshJobs := newAsyncCallbackJobs(func(jobCtx context.Context) {
		if err := publicRemote.Refresh(jobCtx); err != nil && jobCtx.Err() == nil {
			le.WithError(err).Warn("public-read CDN root refresh failed")
		}
	})
	cdnRootRefreshJobs.SetContext(ctx)
	releaseCdnRootChanged := t.a.RegisterCdnRootChangedCallback(func(spaceID string) {
		if spaceID != t.id {
			return
		}
		cdnRootRefreshJobs.Trigger()
	})
	return func() {
		releaseCdnRootChanged()
		cdnRootRefreshJobs.ClearContext()
	}
}

// getChildLists returns the volume's child lists, which order packs, when the
// volume keeps them.
func (t *bstoreTracker) getChildLists() packfile_order.ChildLists {
	if lists, ok := t.a.vol.(packfile_order.ChildLists); ok {
		return lists
	}
	return nil
}

// newCloudOverlay builds the cloud block-store overlay.
func newCloudOverlay(ctx context.Context, le *logrus.Entry, lower, upper block.StoreOps) block.StoreOps {
	return block.NewOverlay(ctx, le, lower, upper, block.OverlayMode_UPPER_WRITE_CACHE, 0, nil)
}

// buildBucketConf builds the bucket config for the block store cache.
func (t *bstoreTracker) buildBucketConf() (*bucket.Config, error) {
	bucketID := BlockStoreBucketID(t.a.accountID, t.id)
	lookupConf, err := bucket.NewLookupConfig(configset.NewControllerConfig(1, &lookup_concurrent.Config{
		NotFoundBehavior:  lookup_concurrent.NotFoundBehavior_NotFoundBehavior_NONE,
		PutBlockBehavior:  lookup_concurrent.PutBlockBehavior_PutBlockBehavior_ALL,
		WritebackBehavior: lookup_concurrent.WritebackBehavior_WritebackBehavior_ALL,
	}))
	if err != nil {
		return nil, err
	}
	return bucket.NewConfig(bucketID, blockStoreBucketConfigRev, lookupConf)
}

// sourceTrackingStore records the source that satisfied completed block reads.
type sourceTrackingStore struct {
	block.StoreOps
	// telemetry receives source observations for bstoreID.
	telemetry *synctelemetry.Store
	// bstoreID identifies the block store whose read source is recorded.
	bstoreID string
	// source identifies reads not already present in an upper cache.
	source     SyncTelemetryBlockSource
	upperCache bool
	// demandStarted and demandFinished bracket each demand read.
	demandStarted  func()
	demandFinished func()
}

// GetStoreParts returns the observed store.
func (s *sourceTrackingStore) GetStoreParts() []block.StorePart {
	return []block.StorePart{{Label: "inner", Store: s.StoreOps}}
}

// BeginReadOperation preserves source observation inside a scoped read.
func (s *sourceTrackingStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scoped, release, err := s.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return s.with(scoped), release, nil
}

// with returns a tracker of the same source over ops.
func (s *sourceTrackingStore) with(ops block.StoreOps) *sourceTrackingStore {
	scoped := *s
	scoped.StoreOps = ops
	return &scoped
}

// HoldsRoots reports whether the observed store holds roots.
func (s *sourceTrackingStore) HoldsRoots() bool { return block.HoldsRoots(s.StoreOps) }

// PinRoot pins a root on the observed store.
func (s *sourceTrackingStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, s.StoreOps, ref)
}

// OpenStage opens a stage on the observed store and observes its reads.
func (s *sourceTrackingStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	staged, release, err := block.OpenStage(ctx, s.StoreOps)
	if err != nil {
		return nil, nil, err
	}
	return s.with(staged), release, nil
}

// ReleaseRoots drops a stage's holds on the observed store.
func (s *sourceTrackingStore) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, s.StoreOps, refs)
}

// MarkRootsComplete records completion proofs in the observed store.
func (s *sourceTrackingStore) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, s.StoreOps, roots)
}

// RootsComplete reads completion proofs from the observed store.
func (s *sourceTrackingStore) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.RootsComplete(ctx, s.StoreOps, refs)
}

// HoldCompleteRoots holds proved roots under the observed store's stage.
func (s *sourceTrackingStore) HoldCompleteRoots(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.HoldCompleteRoots(ctx, s.StoreOps, refs)
}

// GetBlock records the cache, direct, or Cloud source of a successful read.
func (s *sourceTrackingStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	var data []byte
	found, err := s.track(ctx, ref, func() (bool, error) {
		var found bool
		var err error
		data, found, err = s.StoreOps.GetBlock(ctx, ref)
		return found, err
	})
	return data, found, err
}

// GetStoredBlock records the cache, direct, or Cloud source of a successful
// read.
func (s *sourceTrackingStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	var stored *block.StoredBlock
	_, err := s.track(ctx, ref, func() (bool, error) {
		var err error
		stored, err = s.StoreOps.GetStoredBlock(ctx, ref)
		return stored != nil, err
	})
	return stored, err
}

// GetStoredBlockBatch reads refs from the observed store as one batch inside
// one demand window, and records the source of each block it finds as
// GetStoredBlock does.
func (s *sourceTrackingStore) GetStoredBlockBatch(ctx context.Context, refs []*block.BlockRef, fn block.StoredBlockFunc) error {
	// Note which blocks the upper cache already held.
	var cached []bool
	if s.upperCache {
		var err error
		cached, err = s.GetBlockExistsBatch(ctx, refs)
		if err != nil {
			return err
		}
	}

	// Read the blocks inside the demand window.
	if s.demandStarted != nil {
		s.demandStarted()
		defer func() {
			if s.demandFinished != nil {
				s.demandFinished()
			}
		}()
	}

	// Record where each block came from.
	return block.GetStoredBlockBatch(ctx, s.StoreOps, refs, func(i int, stored *block.StoredBlock, err error) error {
		if err == nil && stored != nil {
			source := s.source
			if cached != nil && cached[i] {
				source = SyncTelemetryBlockSourceCache
			}
			s.telemetry.RecordBlockSource(s.bstoreID, source)
		}
		return fn(i, stored, err)
	})
}

// track runs one demand read and records its source when it finds the block.
func (s *sourceTrackingStore) track(ctx context.Context, ref *block.BlockRef, read func() (bool, error)) (bool, error) {
	// Note whether the upper cache already held the block.
	cached := false
	if s.upperCache {
		var err error
		cached, err = s.GetBlockExists(ctx, ref)
		if err != nil {
			return false, err
		}
	}

	// Read the block inside the demand window.
	if s.demandStarted != nil {
		s.demandStarted()
		defer func() {
			if s.demandFinished != nil {
				s.demandFinished()
			}
		}()
	}
	found, err := read()
	if err != nil || !found {
		return found, err
	}

	// Record where the block came from.
	source := s.source
	if cached {
		source = SyncTelemetryBlockSourceCache
	}
	s.telemetry.RecordBlockSource(s.bstoreID, source)
	return true, nil
}

// BuildBlockStoreOpener builds a packfile Opener for a given block store ID
// that reads packs through the current session.
func (a *ProviderAccount) BuildBlockStoreOpener(bstoreID string) packfile_store.Opener {
	return func(packID string, size int64) (*packfile_store.PackReader, error) {
		cli := a.currentSessionClient()
		if cli == nil {
			return nil, errors.New("session client not available")
		}
		return cli.OpenPackReader(bstoreID, packID, size)
	}
}

// buildOpener builds an Opener for packfile HTTP range readers.
func (t *bstoreTracker) buildOpener() packfile_store.Opener {
	return t.a.BuildBlockStoreOpener(t.id)
}

// buildLowerStore selects anonymous reads of a public Space's published
// files or authenticated cloud pack reads.
func (t *bstoreTracker) buildLowerStore(
	ctx context.Context,
	cache packfile_store.IndexCache,
	decodedBlocks *block.DecodedBlockCache,
) (*packfile_store.PackfileStore, *publicReadRemote) {
	if base := t.publicBaseURL(ctx); base != "" {
		remote := newPublicReadRemote(t.a.p.httpCli, base, t.id, cache, decodedBlocks)
		return remote.lower, remote
	}
	return packfile_store.NewPackfileStore(t.buildOpener(), cache, nil), nil
}

// publicBaseURL returns the base URL of the block store's published files
// when it belongs to a public Space, or empty.
func (t *bstoreTracker) publicBaseURL(ctx context.Context) string {
	metadata, err := t.a.GetSharedObjectMetadata(ctx, t.id)
	if err != nil || !metadata.GetPublicRead() || metadata.GetObjectType() != space.SpaceBodyType {
		return ""
	}
	return metadata.GetPublicBaseUrl()
}

// publicReadRemote owns a CDN manifest and its decoded-cache invalidation.
type publicReadRemote struct {
	// root fetches and revalidates the anonymous CDN root pointer.
	root          *cdn_bstore.RootPointerFetcher
	lower         *packfile_store.PackfileStore
	decodedBlocks *block.DecodedBlockCache

	// mtx protects the published manifest snapshot.
	mtx     sync.Mutex
	entries []*packfile.PackfileEntry
}

// newPublicReadRemote builds an anonymous pack store with an initially empty manifest.
func newPublicReadRemote(
	cli *http.Client,
	cdnBaseURL string,
	spaceID string,
	cache packfile_store.IndexCache,
	decodedBlocks *block.DecodedBlockCache,
) *publicReadRemote {
	// Retain the remote's shared cache and manifest publication state.
	remote := &publicReadRemote{
		root:          cdn_bstore.NewRootPointerFetcher(cli, cdnBaseURL, spaceID),
		decodedBlocks: decodedBlocks,
	}

	// Resolve immutable pack IDs through anonymous HTTP range readers.
	remote.lower = packfile_store.NewPackfileStore(
		cdn_bstore.NewAnonymousOpener(cli, cdnBaseURL, spaceID),
		cache, nil)
	return remote
}

// Refresh fetches the anonymous CDN root pointer and updates the lower store.
func (r *publicReadRemote) Refresh(ctx context.Context) error {
	// Fetch the published root pointer.
	ptr, err := r.root.Fetch(ctx)
	if err != nil {
		return err
	}
	entries := clonePackfileEntries(ptr.GetPacks())

	// Invalidate before publishing the refreshed manifest. Reads can observe the
	// lower store and Entries snapshot from separate sources, and stale decoded
	// blocks must not survive into either new view.
	r.decodedBlocks.InvalidateAll(ctx)
	r.lower.UpdateManifest(entries)
	r.mtx.Lock()
	r.entries = entries
	r.mtx.Unlock()

	// Repeat after publication so a read that crossed the old lower manifest
	// cannot store decoded entries with a token from the first invalidation.
	r.decodedBlocks.InvalidateAll(ctx)
	return nil
}

// Entries returns the latest anonymous CDN manifest snapshot.
func (r *publicReadRemote) Entries() []*packfile.PackfileEntry {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return clonePackfileEntries(r.entries)
}

// clonePackfileEntries returns an independent snapshot of non-nil entries.
func clonePackfileEntries(entries []*packfile.PackfileEntry) []*packfile.PackfileEntry {
	out := make([]*packfile.PackfileEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		out = append(out, entry.CloneVT())
	}
	return out
}

// createBlockStore creates a new bstore ref.
func (a *ProviderAccount) createBlockStore(_ context.Context, id string) (*bstore.BlockStoreRef, error) {
	bstoreRef := NewBlockStoreRef(a.conf.GetProviderId(), a.accountID, id)
	if err := bstoreRef.Validate(); err != nil {
		return nil, err
	}
	return bstoreRef, nil
}

// CreateBlockStore creates a new bstore with the given details.
func (a *ProviderAccount) CreateBlockStore(ctx context.Context, id string) (*bstore.BlockStoreRef, error) {
	return a.createBlockStore(ctx, id)
}

// MountBlockStore attempts to mount a BlockStore returning the bstore and a release function.
func (a *ProviderAccount) MountBlockStore(ctx context.Context, ref *bstore.BlockStoreRef, released func()) (bstore.BlockStore, func(), error) {
	// Reject an invalid ref before referencing a tracker.
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}

	// Reference the tracker and wait for its mounted handle.
	tkrRef, tkr, _ := a.bstores.AddKeyRef(ref.GetProviderResourceRef().GetId())
	bs, err := tkr.bstoreCtr.WaitValue(ctx, tkr.errCh)
	if err != nil {
		tkrRef.Release()
		return nil, nil, err
	}
	return bs, tkrRef.Release, nil
}

// GetRoot reads the root from the inner store.
func (b *BlockStore) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return b.store.GetRoot(ctx, name)
}

// ListRoots lists the roots of the inner store.
func (b *BlockStore) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return b.store.ListRoots(ctx, prefix, fn)
}

// SetRoots moves the roots on the inner store.
func (b *BlockStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return b.store.SetRoots(ctx, entries, roots)
}

// HoldsRoots reports whether the local cache bucket below holds roots. It
// holds the Space's roots there: every write lands in that bucket first, and
// only there can a local collector free it.
func (b *BlockStore) HoldsRoots() bool { return block.HoldsRoots(b.store) }

// PinRoot pins a root in the local cache bucket.
func (b *BlockStore) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, b.store, ref)
}

// OpenStage returns a handle whose writes a stage in the local cache bucket
// holds, sharing the cache, sync and retention state.
func (b *BlockStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	// Open the stage in the cache bucket.
	staged, release, err := block.OpenStage(ctx, b.store)
	if err != nil {
		return nil, nil, err
	}

	// Swap the store for the staged one in a copy of the handle.
	scoped := *b
	scoped.store = staged.(block_store.Store)
	return &scoped, release, nil
}

// ReleaseRoots drops a stage's holds in the local cache bucket.
func (b *BlockStore) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	return block.ReleaseRoots(ctx, b.store, refs)
}

// MarkRootsComplete records completion proofs in the local cache bucket.
func (b *BlockStore) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return block.MarkRootsComplete(ctx, b.store, roots)
}

// RootsComplete reads completion proofs from the local cache bucket.
func (b *BlockStore) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.RootsComplete(ctx, b.store, refs)
}

// HoldCompleteRoots holds proved roots under the local cache bucket's stage.
func (b *BlockStore) HoldCompleteRoots(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return block.HoldCompleteRoots(ctx, b.store, refs)
}

// Interface assertions.
var (
	_ bstore.BlockStoreProvider = (*ProviderAccount)(nil)
	_ bstore.BlockStore         = (*BlockStore)(nil)
	_ block.StoreOps            = (*BlockStore)(nil)
	_ block.StoreParts          = (*BlockStore)(nil)
	_ block.LeasedRoots         = (*BlockStore)(nil)
	_ block.RootProofs          = (*BlockStore)(nil)
)
