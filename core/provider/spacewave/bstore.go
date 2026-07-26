package provider_spacewave

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	"github.com/s4wave/spacewave/core/provider"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	packfile_order "github.com/s4wave/spacewave/core/provider/spacewave/packfile/order"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	"github.com/s4wave/spacewave/db/bucket"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	"github.com/s4wave/spacewave/db/volume"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

const (
	httpReaderAtReadAheadSize = 1 * 1024 * 1024
	httpReaderPageSize        = 4 * 1024
	forceSyncTimeout          = 30 * time.Second
)

// blockStoreBucketConfigRev 2 keeps the account-owned cache local-only.
// SessionTransport child buses provide the optional direct lookup layer.
const blockStoreBucketConfigRev = 2

type decodedBlockRefInvalidator interface {
	InvalidateDecodedBlockRef(context.Context, *block.BlockRef)
}

type publicReadRemoteRefresher interface {
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
	// refreshRemote pulls remote packfile metadata into the local read manifest.
	refreshRemote func(ctx context.Context) error
	// remoteSequence returns the local manifest's last-seen remote sequence.
	remoteSequence func(ctx context.Context) (uint64, error)
	// cacheStore is the account-owned non-dirty local cache read/write owner.
	cacheStore block.StoreOps
	// cloudStore is the account-owned Cloud read owner.
	cloudStore block.StoreOps
}

// GetID returns the inner store id.
func (b *BlockStore) GetID() string {
	return b.store.GetID()
}

// GetHashType returns the inner store hash type.
func (b *BlockStore) GetHashType() hash.HashType {
	return b.store.GetHashType()
}

// GetSupportedFeatures returns the native feature bitset for the inner store.
func (b *BlockStore) GetSupportedFeatures() block.StoreFeature {
	return b.store.GetSupportedFeatures()
}

// GetDecodedBlockCache returns the lifecycle-owned decoded-block cache.
func (b *BlockStore) GetDecodedBlockCache() *block.DecodedBlockCache {
	return b.decodedBlocks
}

// InvalidateDecodedBlockRef removes decoded-cache entries for ref.
func (b *BlockStore) InvalidateDecodedBlockRef(ctx context.Context, ref *block.BlockRef) {
	b.decodedBlocks.InvalidateRef(ctx, ref)
}

// BeginReadOperation opens a read scope on the inner store.
func (b *BlockStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := b.store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	scopedStore, ok := store.(block_store.Store)
	if !ok {
		scopedStore = block_store.NewStore(b.store.GetID(), store)
	}
	return &BlockStore{
		store:          scopedStore,
		decodedBlocks:  b.decodedBlocks,
		forceSync:      b.forceSync,
		refreshRemote:  b.refreshRemote,
		remoteSequence: b.remoteSequence,
	}, release, nil
}

// PutBlock forwards to the inner store.
func (b *BlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return b.store.PutBlock(ctx, data, opts)
}

// PutBlockBatch forwards batched writes to the inner store.
func (b *BlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	// Tombstone publication and decoded-cache ownership are separate lower
	// owners; invalidate on both sides so in-flight decoded stores cannot cross
	// the mutation boundary.
	b.invalidateBatchTombstones(ctx, entries)
	if err := b.store.PutBlockBatch(ctx, entries); err != nil {
		return err
	}
	b.invalidateBatchTombstones(ctx, entries)
	return nil
}

// GetBlock forwards to the inner store.
func (b *BlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return b.store.GetBlock(ctx, ref)
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
	// Delete publication and decoded-cache ownership are separate lower owners;
	// invalidate on both sides so in-flight decoded stores cannot cross the
	// mutation boundary.
	b.InvalidateDecodedBlockRef(ctx, ref)
	if err := b.store.RmBlock(ctx, ref); err != nil {
		return err
	}
	b.InvalidateDecodedBlockRef(ctx, ref)
	return nil
}

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

// BeginDeferFlush forwards the GC ref-batch scope to the inner store.
func (b *BlockStore) BeginDeferFlush() {
	block.BeginDeferFlush(b.store)
}

// EndDeferFlush forwards the GC ref-batch scope to the inner store.
func (b *BlockStore) EndDeferFlush(ctx context.Context) error {
	return block.EndDeferFlush(ctx, b.store)
}

// ForceSync flushes any pending dirty blocks to the cloud immediately.
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
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		return errors.Wrap(err, "building decoded block cache")
	}
	defer decodedBlocks.Close()

	// Build lower store (read-only, packfile-backed).
	lower, publicRemote := t.buildLowerStore(ctx, idxCache, decodedBlocks)
	lower.UpdateManifest(mfst.GetEntries())
	releaseSyncTelemetry := t.a.registerSyncTelemetryStore(t.id, lower)
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
		account:    t.a,
		bstoreID:   t.id,
		source:     SyncTelemetryBlockSourceDirect,
		upperCache: true,
	}
	sourceLower := &sourceTrackingStore{
		StoreOps: lower,
		account:  t.a,
		bstoreID: t.id,
		source:   SyncTelemetryBlockSourceCloud,
	}

	// Wrap upper with dirty tracking for the sync controller.
	dirtyUpper := &dirtyTrackingStore{store: sourceUpper}

	localID := BlockStoreID(accountID, t.id)
	overlay := newCloudOverlay(ctx, le, sourceLower, dirtyUpper)

	bstoreHandle := &BlockStore{
		store:         block_store.NewStore(localID, overlay),
		decodedBlocks: decodedBlocks,
		cacheStore:    sourceUpper,
		cloudStore:    sourceLower,
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

	// Build and start the sync controller.
	syncConf := t.a.conf.GetSync()
	sc := &syncController{
		le:         le.WithField("component", "sync"),
		store:      objStore,
		client:     t.a.sessionClient,
		resourceID: t.id,
		mfst:       mfst,
		lower:      lower,
		remote:     nil,
		upper:      upper,
		refGraph:   t.getRefGraph(),
		conf:       syncConf,
		tmpDir:     syncTmpDir(),
		telemetry:  t.a,
		gateBcast:  &t.a.accountBcast,
		skipPull:   publicRemote != nil,
	}
	sc.remotePullRoutine = newCoalescedTriggerRoutine(
		le,
		"bstore-remote-pull",
		sc.pullRemoteOnTrigger,
	)
	if publicRemote != nil {
		sc.remote = publicRemote.Entries
	}

	// Wire dirty tracking from PutBlock to syncController.
	dirtyUpper.markDirty = sc.MarkDirty
	bstoreHandle.forceSync = sc.FlushNowUnordered
	bstoreHandle.refreshRemote = sc.PullNow
	bstoreHandle.remoteSequence = sc.LastPullSequence

	if !sc.skipPull && t.a.wsTracker != nil {
		t.a.wsTracker.RegisterBlockStoreNonceCallback(t.id, func(uint64) {
			sc.TriggerRemotePull()
		})
		defer t.a.wsTracker.UnregisterBlockStoreNonceCallback(t.id)
	}

	// Run the initial pull. If access is gated, signal the error to mount
	// callers and block to prevent keyed retry.
	if err := sc.Init(ctx); err != nil {
		if isCloudAccessGatedError(err) {
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
	remoteSequence, err := sc.LastPullSequence(ctx)
	if err != nil {
		return errors.Wrap(err, "reading initial cloud remote sequence")
	}
	t.a.setSyncTelemetryCloudRemoteSequence(t.id, remoteSequence)

	syncOwner := newBstoreSyncOwner(le, sc)
	syncOwner.Start(ctx)
	defer syncOwner.Stop()

	// Done, publish the block store.
	le.Debug("mounted cloud bstore")
	t.bstoreCtr.SetValue(bstoreHandle)

	<-ctx.Done()

	t.bstoreCtr.SetValue(nil)
	return context.Canceled
}

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

func (t *bstoreTracker) getRefGraph() packfile_order.RefGraph {
	if kvVol, ok := t.a.vol.(kvtx_volume.KvtxVolume); ok {
		return kvVol.GetRefGraph()
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
	account        *ProviderAccount
	bstoreID       string
	source         SyncTelemetryBlockSource
	upperCache     bool
	demandStarted  func()
	demandFinished func()
}

// BeginReadOperation preserves source observation inside a scoped read.
func (s *sourceTrackingStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scoped, release, err := s.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &sourceTrackingStore{
		StoreOps:       scoped,
		account:        s.account,
		bstoreID:       s.bstoreID,
		source:         s.source,
		upperCache:     s.upperCache,
		demandStarted:  s.demandStarted,
		demandFinished: s.demandFinished,
	}, release, nil
}

// GetBlock records cache, direct, or Cloud ownership for a successful read.
func (s *sourceTrackingStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	cached := false
	if s.upperCache {
		var err error
		cached, err = s.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, false, err
		}
	}
	if s.demandStarted != nil {
		s.demandStarted()
		defer func() {
			if s.demandFinished != nil {
				s.demandFinished()
			}
		}()
	}
	data, found, err := s.StoreOps.GetBlock(ctx, ref)
	if err != nil || !found {
		return data, found, err
	}
	source := s.source
	if cached {
		source = SyncTelemetryBlockSourceCache
	}
	s.account.recordSyncTelemetryBlockSource(s.bstoreID, source)
	return data, true, nil
}

// dirtyTrackingStore wraps block.StoreOps and calls markDirty on new PutBlock.
type dirtyTrackingStore struct {
	store     block.StoreOps
	markDirty func(ctx context.Context, h *hash.Hash, size int64)
}

// GetHashType returns the inner store hash type.
func (d *dirtyTrackingStore) GetHashType() hash.HashType {
	return d.store.GetHashType()
}

// GetSupportedFeatures returns the inner store native feature bitset.
func (d *dirtyTrackingStore) GetSupportedFeatures() block.StoreFeature {
	return d.store.GetSupportedFeatures()
}

// BeginReadOperation opens a read scope on the inner store.
func (d *dirtyTrackingStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := d.store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &dirtyTrackingStore{store: store, markDirty: d.markDirty}, release, nil
}

// PutBlock puts a block and marks it dirty if new.
func (d *dirtyTrackingStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := d.store.PutBlock(ctx, data, opts)
	if err == nil && !existed && d.markDirty != nil {
		d.markDirty(ctx, ref.GetHash(), int64(len(data)))
	}
	return ref, existed, err
}

// PutBlockBatch writes blocks and marks successful new entries dirty.
func (d *dirtyTrackingStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	var exists []bool
	var valid []int
	if d.markDirty != nil {
		refs := make([]*block.BlockRef, 0, len(entries))
		for i, entry := range entries {
			if entry == nil || entry.Tombstone || entry.Ref == nil || entry.Ref.GetEmpty() {
				continue
			}
			valid = append(valid, i)
			refs = append(refs, entry.Ref)
		}
		var err error
		exists, err = d.store.GetBlockExistsBatch(ctx, refs)
		if err != nil || len(exists) != len(refs) {
			// Existence preflight is advisory; fall back to conservative dirty
			// marking rather than failing the underlying batch write.
			exists = nil
		}
	}
	if err := d.store.PutBlockBatch(ctx, entries); err != nil {
		return err
	}
	if d.markDirty != nil {
		for j, i := range valid {
			if exists != nil && exists[j] {
				continue
			}
			entry := entries[i]
			d.markDirty(ctx, entry.Ref.GetHash(), int64(len(entry.Data)))
		}
	}
	if invalidator, ok := d.store.(decodedBlockRefInvalidator); ok {
		// Dirty-tracking batches can wrap cache-owning stores directly; keep
		// tombstone invalidation with the wrapper that observes the deletion.
		for _, entry := range entries {
			if entry != nil && entry.Tombstone {
				invalidator.InvalidateDecodedBlockRef(ctx, entry.Ref)
			}
		}
	}
	return nil
}

// GetBlock gets a block by reference.
func (d *dirtyTrackingStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return d.store.GetBlock(ctx, ref)
}

// GetBlockExists checks if a block exists.
func (d *dirtyTrackingStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return d.store.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch forwards batched existence probes to the inner store.
func (d *dirtyTrackingStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return d.store.GetBlockExistsBatch(ctx, refs)
}

// RmBlock removes a block.
func (d *dirtyTrackingStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if err := d.store.RmBlock(ctx, ref); err != nil {
		return err
	}
	if invalidator, ok := d.store.(decodedBlockRefInvalidator); ok {
		invalidator.InvalidateDecodedBlockRef(ctx, ref)
	}
	return nil
}

// StatBlock returns block metadata.
func (d *dirtyTrackingStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return d.store.StatBlock(ctx, ref)
}

// Sync forwards the durability barrier to the inner store.
func (d *dirtyTrackingStore) Sync(ctx context.Context) (bool, error) {
	return d.store.Sync(ctx)
}

// BeginDeferFlush forwards the GC ref-batch scope to the inner store.
func (d *dirtyTrackingStore) BeginDeferFlush() {
	block.BeginDeferFlush(d.store)
}

// EndDeferFlush forwards the GC ref-batch scope to the inner store.
func (d *dirtyTrackingStore) EndDeferFlush(ctx context.Context) error {
	return block.EndDeferFlush(ctx, d.store)
}

// BuildBlockStoreOpener builds a packfile Opener for a given block store ID.
// The opener builds shared pack readers backed by signed HTTP Range requests.
// The size is taken from the manifest entry, so no HEAD request is issued.
func (a *ProviderAccount) BuildBlockStoreOpener(bstoreID string) packfile_store.Opener {
	return func(packID string, size int64) (*packfile_store.PackReader, error) {
		if size <= 0 {
			return nil, errors.New("pack size must be known from the manifest")
		}
		url := a.p.endpoint + "/api/bstore/" + bstoreID + "/pack/" + packID
		return packfile_store.NewHTTPRangeReader(
			a.p.httpCli,
			url,
			size,
			httpReaderAtReadAheadSize,
			httpReaderPageSize,
			func(req *http.Request) error {
				return a.sessionClient.signPackReadRequest(req, bstoreID)
			},
			func(resp *http.Response) {
				a.sessionClient.observePackReadResponse(bstoreID, resp)
			},
		), nil
	}
}

// buildOpener builds an Opener for packfile HTTP range readers.
func (t *bstoreTracker) buildOpener() packfile_store.Opener {
	return t.a.BuildBlockStoreOpener(t.id)
}

func (t *bstoreTracker) buildLowerStore(
	ctx context.Context,
	cache packfile_store.IndexCache,
	decodedBlocks *block.DecodedBlockCache,
) (*packfile_store.PackfileStore, *publicReadRemote) {
	if t.isPublicReadSpaceBlockStore(ctx) {
		remote := newPublicReadRemote(t.a.p.httpCli, cdn.BaseURL(), t.id, cache, decodedBlocks)
		return remote.lower, remote
	}
	return packfile_store.NewPackfileStore(t.buildOpener(), cache), nil
}

func (t *bstoreTracker) isPublicReadSpaceBlockStore(ctx context.Context) bool {
	metadata, err := t.a.GetSharedObjectMetadata(ctx, t.id)
	if err != nil {
		return false
	}
	return metadata.GetPublicRead() && metadata.GetObjectType() == space.SpaceBodyType
}

type publicReadRemote struct {
	cli           *http.Client
	cdnBaseURL    string
	spaceID       string
	lower         *packfile_store.PackfileStore
	decodedBlocks *block.DecodedBlockCache

	mtx     sync.Mutex
	entries []*packfile.PackfileEntry
}

func newPublicReadRemote(
	cli *http.Client,
	cdnBaseURL string,
	spaceID string,
	cache packfile_store.IndexCache,
	decodedBlocks *block.DecodedBlockCache,
) *publicReadRemote {
	remote := &publicReadRemote{
		cli:           cli,
		cdnBaseURL:    cdnBaseURL,
		spaceID:       spaceID,
		decodedBlocks: decodedBlocks,
	}
	remote.lower = packfile_store.NewPackfileStore(
		cdn_bstore.NewAnonymousOpener(cli, cdnBaseURL, spaceID),
		cache,
	)
	return remote
}

// Refresh fetches the anonymous CDN root pointer and updates the lower store.
func (r *publicReadRemote) Refresh(ctx context.Context) error {
	ptr, err := cdn_bstore.FetchRootPointer(ctx, r.cli, r.cdnBaseURL, r.spaceID)
	if err != nil {
		return err
	}
	entries := clonePackfileEntries(ptr.GetPacks())
	// Invalidate before publishing the refreshed manifest. Reads can observe the
	// lower store and Entries snapshot from separate owners, and stale decoded
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
	providerID := a.conf.GetProviderId()
	accountID := a.accountID
	bstoreRef := NewBlockStoreRef(providerID, accountID, id)
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
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}

	bstoreID := ref.GetProviderResourceRef().GetId()
	tkrRef, tkr, _ := a.bstores.AddKeyRef(bstoreID)

	bs, err := tkr.bstoreCtr.WaitValue(ctx, tkr.errCh)
	if err != nil {
		tkrRef.Release()
		return nil, nil, err
	}

	return bs, tkrRef.Release, nil
}

// EnumerateBlockRefs returns all block refs from the cloud block store by pulling
// the packfile manifest and scanning each packfile's index entries.
func (a *ProviderAccount) EnumerateBlockRefs(ctx context.Context, bstoreID string) ([]*block.BlockRef, error) {
	// Pull all packfile entries from the cloud.
	pullData, err := a.sessionClient.SyncPull(ctx, bstoreID, "")
	if err != nil {
		return nil, errors.Wrap(err, "sync pull")
	}
	if len(pullData) == 0 {
		return nil, nil
	}

	resp := &packfile.PullResponse{}
	if err := resp.UnmarshalJSON(pullData); err != nil {
		return nil, errors.Wrap(err, "unmarshal pull response")
	}

	entries := resp.GetEntries()
	if len(entries) == 0 {
		return nil, nil
	}

	// Build an opener for this block store.
	opener := a.BuildBlockStoreOpener(bstoreID)

	// For each packfile, open it and enumerate all block hashes from the index.
	var refs []*block.BlockRef
	for _, entry := range entries {
		size := int64(entry.GetSizeBytes())
		if size <= 0 {
			continue
		}
		rd, err := opener(entry.GetId(), size)
		if err != nil {
			return nil, errors.Wrapf(err, "open packfile %s", entry.GetId())
		}
		ra := rd.ReaderAt(ctx)

		reader, err := kvfile.BuildReader(ra, uint64(size))
		if err != nil {
			return nil, errors.Wrapf(err, "build reader for packfile %s", entry.GetId())
		}

		err = reader.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
			h := &hash.Hash{}
			if err := h.ParseFromB58(string(ie.GetKey())); err != nil {
				return nil
			}
			refs = append(refs, block.NewBlockRef(h))
			return nil
		})
		if err != nil {
			return nil, errors.Wrapf(err, "scan index entries for packfile %s", entry.GetId())
		}
	}

	return refs, nil
}

// _ is a type assertion
var (
	_ bstore.BlockStoreProvider = ((*ProviderAccount)(nil))
	_ bstore.BlockStore         = ((*BlockStore)(nil))
	_ block.StoreOps            = ((*BlockStore)(nil))
	_ block.StoreOps            = ((*dirtyTrackingStore)(nil))
)
