package provider_local

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	"github.com/s4wave/spacewave/db/bucket"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// blockStoreBucketConfigRev 2 keeps local-provider buckets local-only.
const blockStoreBucketConfigRev = 2

// BlockStore implements the bstore interface.
type BlockStore struct {
	// store owns local writes and durability.
	store block_store.Store
	// readStore optionally routes uncached reads through the active Session DEX.
	readStore block_store.Store
	// decodedBlocks is the decoded-block cache owned by the block-store lifecycle.
	decodedBlocks *block.DecodedBlockCache
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

func (b *BlockStore) readOwner() block_store.Store {
	if b.readStore != nil {
		return b.readStore
	}
	return b.store
}

// BeginReadOperation opens a read scope on the inner store.
func (b *BlockStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	owner := b.readOwner()
	store, release, err := owner.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	scopedStore, ok := store.(block_store.Store)
	if !ok {
		scopedStore = block_store.NewStore(owner.GetID(), store)
	}
	if b.readStore != nil {
		return &BlockStore{
			store:         b.store,
			readStore:     scopedStore,
			decodedBlocks: b.decodedBlocks,
		}, release, nil
	}
	return &BlockStore{store: scopedStore, decodedBlocks: b.decodedBlocks}, release, nil
}

// PutBlock forwards to the inner store.
func (b *BlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return b.store.PutBlock(ctx, data, opts)
}

// PutBlockBatch forwards batched writes to the inner store.
func (b *BlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	if err := b.store.PutBlockBatch(ctx, entries); err != nil {
		return err
	}
	// Batch tombstones bypass RmBlock, so the provider wrapper must invalidate
	// decoded entries here before any future read can reuse stale content.
	b.invalidateBatchTombstones(ctx, entries)
	return nil
}

// GetBlock forwards to the active Session read owner when configured.
func (b *BlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return b.readOwner().GetBlock(ctx, ref)
}

// GetBlockExists forwards to the active Session read owner when configured.
func (b *BlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return b.readOwner().GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch forwards batched existence probes to the active read owner.
func (b *BlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return b.readOwner().GetBlockExistsBatch(ctx, refs)
}

// RmBlock forwards to the inner store.
func (b *BlockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
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

// StatBlock forwards to the active Session read owner when configured.
func (b *BlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return b.readOwner().StatBlock(ctx, ref)
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

// NewBlockStoreRef builds a new BlockStoreRef.
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
	// a is the provider account
	a *ProviderAccount
	// id is the bstore id
	id string
	// bstoreCtr is the bstore container
	bstoreCtr *ccontainer.CContainer[*BlockStore]
}

// buildBlockStoreTracker builds a new bstoreTracker for a bstore id.
func (a *ProviderAccount) buildBlockStoreTracker(bstoreID string) (keyed.Routine, *bstoreTracker) {
	tracker := &bstoreTracker{
		a:         a,
		id:        bstoreID,
		bstoreCtr: ccontainer.NewCContainer[*BlockStore](nil),
	}
	return tracker.executeBlockStoreTracker, tracker
}

// executeBlockStoreTracker exeecutes the bstoreTracker for the bstore.
func (t *bstoreTracker) executeBlockStoreTracker(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	le := t.a.le.WithField("bstore-id", t.id)
	le.Debug("mounting bstore")

	// Local provider: ensure the bucket exists
	bucketConf, err := t.buildBucketConf()
	if err != nil {
		return err
	}

	// Apply bucket config. Note that if there is a config with a higher rev,
	// nothing will happen and the one with the higher revision will be used.
	volID := t.a.vol.GetID()
	applyBucketResult, err := bucket.ExApplyBucketConfig(
		ctx,
		t.a.t.p.b,
		bucket.NewApplyBucketConfigToVolume(bucketConf, volID),
	)
	if err != nil {
		return err
	}
	if errStr := applyBucketResult.GetError(); errStr != "" {
		return errors.New(errStr)
	}

	// Mount the block store.
	blockStoreLocalID := BlockStoreLocalID(
		t.a.t.p.info.GetProviderId(),
		t.a.t.accountInfo.GetProviderAccountId(),
		t.id,
	)
	bucketHandle, _, bucketHandleRef, err := bucket.ExBuildBucketAPI(
		ctx,
		t.a.t.p.b,
		false,
		bucketConf.GetId(),
		volID,
		ctxCancel,
	)
	if err != nil {
		return err
	}
	defer bucketHandleRef.Release()

	// not expected
	if !bucketHandle.GetExists() {
		return errors.New("bucket does not exist even after creating it")
	}

	// Construct the block store handle and controller.
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		return err
	}
	defer decodedBlocks.Close()
	localBucket := bucketHandle.GetBucket()
	bucketID := BlockStoreBucketID(
		t.a.t.p.info.GetProviderId(),
		t.a.t.accountInfo.GetProviderAccountId(),
		t.id,
	)
	localStore := block_store.NewStore(blockStoreLocalID, localBucket)
	readOps := block_store.NewStoreReadThrough(
		func() block.StoreOps { return localBucket },
		func() block.StoreOps { return t.a.getP2PStore(bucketID) },
		true,
	)
	bstoreHandle := &BlockStore{
		store:         localStore,
		readStore:     block_store.NewStore(blockStoreLocalID, readOps),
		decodedBlocks: decodedBlocks,
	}
	bstoreCtrl := newLocalBlockStoreController(le, blockStoreLocalID, localStore)
	relBstoreCtrl, err := t.a.t.p.b.AddController(ctx, bstoreCtrl, nil)
	if err != nil {
		return err
	}
	defer relBstoreCtrl()

	// Done
	le.Debug("mounted bstore successfully")
	t.bstoreCtr.SetValue(bstoreHandle)
	<-ctx.Done()

	t.bstoreCtr.SetValue(nil)
	return context.Canceled
}

func newLocalBlockStoreController(
	le *logrus.Entry,
	blockStoreLocalID string,
	localStore block_store.Store,
) *block_store_controller.Controller {
	return block_store_controller.NewController(
		le,
		controller.NewInfo(ControllerID+"/bstore", Version, "local block store for: "+blockStoreLocalID),
		block_store_controller.NewBlockStoreBuilder(localStore),
		[]string{blockStoreLocalID},
		true,
		[]string{blockStoreLocalID},
		false,
		false,
	)
}

// buildBucketConf builds the bucket config for the bstore.
func (t *bstoreTracker) buildBucketConf() (*bucket.Config, error) {
	bucketID := BlockStoreBucketID(
		t.a.t.p.info.GetProviderId(),
		t.a.t.accountInfo.GetProviderAccountId(),
		t.id,
	)
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

// createBlockStoreLocked creates a new bstore with the given details.
// Assumes a.mtx is locked.
func (a *ProviderAccount) createBlockStoreLocked(ctx context.Context, id string) (*bstore.BlockStoreRef, error) {
	// build the bstore ref
	providerID := a.t.accountInfo.GetProviderId()
	providerAccountID := a.t.accountInfo.GetProviderAccountId()
	bstoreRef := NewBlockStoreRef(providerID, providerAccountID, id)

	// validate the ref (also validates the id)
	if err := bstoreRef.Validate(); err != nil {
		return nil, err
	}

	// TODO: store block store?

	// return the ws ref
	return bstoreRef, nil
}

// CreateBlockStore creates a new bstore with the given details.
func (a *ProviderAccount) CreateBlockStore(ctx context.Context, id string) (*bstore.BlockStoreRef, error) {
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer relMtx()

	return a.createBlockStoreLocked(ctx, id)
}

// MountBlockStore attempts to mount a BlockStore returning the bstore and a release function.
//
// usually called by the provider controller
func (a *ProviderAccount) MountBlockStore(ctx context.Context, ref *bstore.BlockStoreRef, released func()) (bstore.BlockStore, func(), error) {
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}

	bstoreID := ref.GetProviderResourceRef().GetId()
	tkrRef, tkr, _ := a.bstores.AddKeyRef(bstoreID)

	bstore, err := tkr.bstoreCtr.WaitValue(ctx, nil)
	if err != nil {
		tkrRef.Release()
		return nil, nil, err
	}

	return bstore, tkrRef.Release, nil
}

// _ is a type assertion
var (
	_ bstore.BlockStoreProvider = ((*ProviderAccount)(nil))
	_ bstore.BlockStore         = ((*BlockStore)(nil))
	_ block.StoreOps            = ((*BlockStore)(nil))
)
