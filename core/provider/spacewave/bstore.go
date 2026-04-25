package provider_spacewave

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
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
	"github.com/s4wave/spacewave/db/volume"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

const (
	httpReaderAtReadAheadSize = 1 * 1024 * 1024
	httpReaderPageSize        = 4 * 1024
	forceSyncTimeout          = 30 * time.Second
)

// BlockStore wraps a block store overlay with packfile-backed cloud storage.
type BlockStore struct {
	// store is the inner block store overlay.
	store block_store.Store
	// forceSync flushes pending dirty blocks to the cloud immediately.
	forceSync func(ctx context.Context) error
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

// PutBlock forwards to the inner store.
func (b *BlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return b.store.PutBlock(ctx, data, opts)
}

// PutBlockBatch forwards batched writes to the inner store.
func (b *BlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	return b.store.PutBlockBatch(ctx, entries)
}

// PutBlockBackground forwards background writes to the inner store.
func (b *BlockStore) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return b.store.PutBlockBackground(ctx, data, opts)
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
	return b.store.RmBlock(ctx, ref)
}

// StatBlock forwards to the inner store.
func (b *BlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return b.store.StatBlock(ctx, ref)
}

// Flush forwards to the inner store.
func (b *BlockStore) Flush(ctx context.Context) error {
	return b.store.Flush(ctx)
}

// BeginDeferFlush forwards to the inner store.
func (b *BlockStore) BeginDeferFlush() {
	b.store.BeginDeferFlush()
}

// EndDeferFlush forwards to the inner store.
func (b *BlockStore) EndDeferFlush(ctx context.Context) error {
	return b.store.EndDeferFlush(ctx)
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

	// Build lower store (read-only, packfile-backed).
	lower, publicRemote := t.buildLowerStore(ctx, idxCache)
	lower.UpdateManifest(mfst.GetEntries())
	releaseSyncTelemetry := t.a.registerSyncTelemetryStore(t.id, lower)
	defer releaseSyncTelemetry()
	if publicRemote != nil {
		if err := publicRemote.Refresh(ctx); err != nil {
			le.WithError(err).Warn("public-read CDN root refresh failed")
		}
		releaseCdnRootChanged := t.a.RegisterCdnRootChangedCallback(func(spaceID string) {
			if spaceID != t.id {
				return
			}
			go func() {
				if err := publicRemote.Refresh(ctx); err != nil && ctx.Err() == nil {
					le.WithError(err).Warn("public-read CDN root refresh failed")
				}
			}()
		})
		defer releaseCdnRootChanged()
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

	// Wrap upper with dirty tracking for the sync controller.
	dirtyUpper := &dirtyTrackingStore{store: upper}

	// Build overlay: upper-first cache lookups with lower reads handled by the
	// packfile store's own non-dirty persistence path.
	localID := BlockStoreID(accountID, t.id)
	overlay := newCloudOverlay(ctx, lower, dirtyUpper)

	// Build the block store handle.
	bstoreHandle := &BlockStore{store: block_store.NewStore(localID, overlay)}

	// Build and register block store controller on the bus.
	bstoreCtrl := block_store_controller.NewController(
		le,
		controller.NewInfo(ControllerID+"/bstore", Version, "cloud block store for: "+localID),
		block_store_controller.NewBlockStoreBuilder(bstoreHandle.store),
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
	if publicRemote != nil {
		sc.remote = publicRemote.Entries
	}

	// Wire dirty tracking from PutBlock to syncController.
	dirtyUpper.markDirty = sc.MarkDirty
	bstoreHandle.forceSync = sc.FlushNowUnordered

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

	// Run sync loop in a goroutine; cancellation via ctx.
	syncDone := make(chan error, 1)
	go func() {
		syncDone <- sc.Execute(ctx)
	}()

	// Done, publish the block store.
	le.Debug("mounted cloud bstore")
	t.bstoreCtr.SetValue(bstoreHandle)

	select {
	case <-ctx.Done():
	case err := <-syncDone:
		if err != nil {
			le.WithError(err).Warn("sync controller exited with error")
		}
	}

	t.bstoreCtr.SetValue(nil)
	return context.Canceled
}

func (t *bstoreTracker) getRefGraph() packfile_order.RefGraph {
	if kvVol, ok := t.a.vol.(kvtx_volume.KvtxVolume); ok {
		return kvVol.GetRefGraph()
	}
	return nil
}

// newCloudOverlay builds the cloud block-store overlay.
func newCloudOverlay(ctx context.Context, lower, upper block.StoreOps) block.StoreOps {
	return block.NewOverlay(ctx, lower, upper, block.OverlayMode_UPPER_WRITE_CACHE, 0, nil)
}

// buildBucketConf builds the bucket config for the block store cache.
func (t *bstoreTracker) buildBucketConf() (*bucket.Config, error) {
	bucketID := BlockStoreBucketID(t.a.accountID, t.id)
	return bucket.NewConfig(bucketID, 1, nil, &bucket.LookupConfig{})
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

// PutBlock puts a block and marks it dirty if new.
func (d *dirtyTrackingStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := d.store.PutBlock(ctx, data, opts)
	if err == nil && !existed && d.markDirty != nil {
		d.markDirty(ctx, ref.GetHash(), int64(len(data)))
	}
	return ref, existed, err
}

// PutBlockBatch writes blocks and conservatively marks all successful
// non-tombstone writes dirty.
func (d *dirtyTrackingStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	if err := d.store.PutBlockBatch(ctx, entries); err != nil {
		return err
	}
	if d.markDirty != nil {
		for _, entry := range entries {
			if entry.Tombstone || entry.Ref == nil || entry.Ref.GetEmpty() {
				continue
			}
			d.markDirty(ctx, entry.Ref.GetHash(), int64(len(entry.Data)))
		}
	}
	return nil
}

// PutBlockBackground writes a block using the inner background path.
func (d *dirtyTrackingStore) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := d.store.PutBlockBackground(ctx, data, opts)
	if err == nil && !existed && d.markDirty != nil {
		d.markDirty(ctx, ref.GetHash(), int64(len(data)))
	}
	return ref, existed, err
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
	return d.store.RmBlock(ctx, ref)
}

// StatBlock returns block metadata.
func (d *dirtyTrackingStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return d.store.StatBlock(ctx, ref)
}

// Flush flushes the inner store.
func (d *dirtyTrackingStore) Flush(ctx context.Context) error {
	return d.store.Flush(ctx)
}

// BeginDeferFlush forwards to the inner store.
func (d *dirtyTrackingStore) BeginDeferFlush() {
	d.store.BeginDeferFlush()
}

// EndDeferFlush forwards to the inner store.
func (d *dirtyTrackingStore) EndDeferFlush(ctx context.Context) error {
	return d.store.EndDeferFlush(ctx)
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
) (*packfile_store.PackfileStore, *publicReadRemote) {
	if t.isPublicReadSpaceBlockStore(ctx) {
		remote := newPublicReadRemote(t.a.p.httpCli, cdn.BaseURL(), t.id, cache)
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
	cli        *http.Client
	cdnBaseURL string
	spaceID    string
	lower      *packfile_store.PackfileStore

	mtx     sync.Mutex
	entries []*packfile.PackfileEntry
}

func newPublicReadRemote(
	cli *http.Client,
	cdnBaseURL string,
	spaceID string,
	cache packfile_store.IndexCache,
) *publicReadRemote {
	remote := &publicReadRemote{
		cli:        cli,
		cdnBaseURL: cdnBaseURL,
		spaceID:    spaceID,
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
	r.mtx.Lock()
	r.entries = entries
	r.mtx.Unlock()
	r.lower.UpdateManifest(entries)
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
