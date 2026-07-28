package provider_local

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

type batchForwardTestStore struct {
	block_store.Store
	putBlockBatchHits int
	putBlockHits      int
	existsBatchHits   int
}

func newBatchForwardTestStore() *batchForwardTestStore {
	ops := block_store_inmem.NewInmemBlock(
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		0,
		false,
	)
	return &batchForwardTestStore{
		Store: block_store.NewStore("test", ops),
	}
}

func (s *batchForwardTestStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	s.putBlockBatchHits++
	return s.Store.PutBlockBatch(ctx, entries)
}

func (s *batchForwardTestStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.putBlockHits++
	return s.Store.PutBlock(ctx, data, opts)
}

func (s *batchForwardTestStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchHits++
	return s.Store.GetBlockExistsBatch(ctx, refs)
}

var (
	_ block_store.Store = ((*batchForwardTestStore)(nil))
	_ block.StoreOps    = ((*batchForwardTestStore)(nil))
)

type localDEXTestStore struct {
	block.StoreOps
	requests atomic.Int32
}

func (s *localDEXTestStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	s.requests.Add(1)
	return s.StoreOps.GetBlock(ctx, ref)
}

var _ block.StoreOps = ((*localDEXTestStore)(nil))

func TestMountedBlockStoreReadUsesPriorSourceDuringReplacement(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	_, _, acc, sess, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()
	first := acc.GetSessionTransport()
	if first == nil {
		t.Fatal("expected initial session transport")
	}

	data := []byte("from-prior-production-source")
	blockRef, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	priorStore := newBatchForwardTestStore()
	if _, _, err := priorStore.PutBlock(ctx, data, nil); err != nil {
		t.Fatal(err)
	}

	bstoreRef, err := acc.CreateBlockStore(ctx, "replacement-window")
	if err != nil {
		t.Fatal(err)
	}
	bucketID := BlockStoreBucketID(
		acc.GetProviderID(),
		acc.GetAccountID(),
		bstoreRef.GetProviderResourceRef().GetId(),
	)
	priorCtx, priorCancel := context.WithCancel(ctx)
	defer priorCancel()
	prior := &p2pSyncState{
		ctx:              priorCtx,
		cancel:           priorCancel,
		sessionTransport: first,
		startComplete:    true,
		started:          true,
		startupExited:    true,
		owners:           1,
		stores:           map[string]block.StoreOps{bucketID: priorStore},
	}
	acc.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		acc.p2pSync = prior
		bcast()
	})

	if err := acc.CreateSessionTransport(ctx, sess.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	second := acc.GetSessionTransport()
	if second == nil || second == first {
		t.Fatal("expected replacement session transport")
	}

	loadStarted := make(chan struct{})
	releaseLoad := make(chan struct{})
	var gate atomic.Bool
	removeHandler, err := second.GetChildBus().AddHandler(directive.NewFuncHandler(
		func(handlerCtx context.Context, di directive.Instance) ([]directive.Resolver, error) {
			load, ok := di.GetDirective().(resolver.LoadControllerWithConfig)
			if !ok {
				return nil, nil
			}
			if _, ok := load.GetLoadControllerConfig().(*dex_solicit.Config); !ok {
				return nil, nil
			}
			if gate.CompareAndSwap(false, true) {
				close(loadStarted)
				select {
				case <-releaseLoad:
				case <-handlerCtx.Done():
				}
			}
			return nil, nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	defer removeHandler()

	mounted, releaseMounted, err := acc.MountBlockStore(ctx, bstoreRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseMounted()

	startDone := make(chan error, 1)
	go func() {
		startDone <- acc.StartP2PSync(ctx, second)
	}()
	select {
	case <-loadStarted:
	case <-ctx.Done():
		t.Fatal("replacement controller load did not start")
	}
	acc.releaseP2PSyncState(prior)
	prior.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if prior.stopping {
			t.Fatal("prior P2P source stopped while replacement was starting")
		}
	})

	got, found, err := mounted.GetBlock(ctx, blockRef)
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("production replacement lower source = %q/%v/%v", got, found, err)
	}

	close(releaseLoad)
	if err := <-startDone; err != nil {
		t.Fatal(err)
	}
	var current *p2pSyncState
	acc.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current = acc.p2pSync
	})
	current.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if current.lowerSource != nil || current.lowerSourceHeld {
			t.Fatal("replacement retained its lower P2P source after startup")
		}
	})
}

func TestLocalBlockStoreMissRoutesToSessionChildDEX(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	data := []byte("from-session-dex")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}

	remote := newBatchForwardTestStore()
	if _, _, err := remote.PutBlock(ctx, data, nil); err != nil {
		t.Fatal(err)
	}
	dexStore := &localDEXTestStore{StoreOps: remote}
	local := newBatchForwardTestStore()
	readOps := block_store.NewStoreReadThrough(
		func() block.StoreOps { return local },
		func() block.StoreOps { return dexStore },
		true,
	)
	store := &BlockStore{
		store:     block_store.NewStore("local-store", local),
		readStore: block_store.NewStore("local-store", readOps),
	}

	got, found, err := store.GetBlock(ctx, ref)
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("local DEX fallback = %q/%v/%v", got, found, err)
	}
	if dexStore.requests.Load() != 1 {
		t.Fatalf("local DEX requests = %d, want 1", dexStore.requests.Load())
	}
	if local.putBlockHits != 1 {
		t.Fatalf("local cache writes = %d, want 1", local.putBlockHits)
	}

	got, found, err = store.GetBlock(ctx, ref)
	if err != nil || !found || string(got) != string(data) {
		t.Fatalf("local cache hit = %q/%v/%v", got, found, err)
	}
	if dexStore.requests.Load() != 1 {
		t.Fatalf("local DEX requests after cache hit = %d, want 1", dexStore.requests.Load())
	}
}

func TestBlockStoreBucketUsesLocalOnlyLookup(t *testing.T) {
	tracker := &bstoreTracker{
		a: &ProviderAccount{
			t: &providerAccountTracker{
				p: &Provider{
					info: &provider.ProviderInfo{ProviderId: "local"},
				},
				accountInfo: &provider.ProviderAccountInfo{
					ProviderAccountId: "account",
				},
			},
		},
		id: "block-store",
	}

	conf, err := tracker.buildBucketConf()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := conf.GetId(), BlockStoreBucketID("local", "account", "block-store"); got != want {
		t.Fatalf("bucket id = %q, want %q", got, want)
	}
	if got := conf.GetRev(); got != blockStoreBucketConfigRev {
		t.Fatalf("bucket config revision = %d, want %d", got, blockStoreBucketConfigRev)
	}

	var lookupConf lookup_concurrent.Config
	if err := lookupConf.UnmarshalJSON(conf.GetLookup().GetController().GetConfig()); err != nil {
		t.Fatal(err)
	}
	if got := lookupConf.GetNotFoundBehavior(); got != lookup_concurrent.NotFoundBehavior_NotFoundBehavior_NONE {
		t.Fatalf("not-found behavior = %s, want local-only lookup", got.String())
	}
	if got := lookupConf.GetPutBlockBehavior(); got != lookup_concurrent.PutBlockBehavior_PutBlockBehavior_ALL {
		t.Fatalf("put behavior = %s, want all local handles", got.String())
	}
	if got := lookupConf.GetWritebackBehavior(); got != lookup_concurrent.WritebackBehavior_WritebackBehavior_ALL {
		t.Fatalf("writeback behavior = %s, want all local handles", got.String())
	}
}

func TestBlockStoreForwardsNativeOperations(t *testing.T) {
	ctx := context.Background()
	inner := newBatchForwardTestStore()
	store := &BlockStore{store: inner}
	batchData := []byte("batch")
	batchRef, err := block.BuildBlockRef(batchData, &block.PutOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}

	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: batchRef, Data: batchData}}); err != nil {
		t.Fatalf("PutBlockBatch failed: %v", err)
	}
	if inner.putBlockBatchHits != 1 {
		t.Fatalf("expected 1 PutBlockBatch call, got %d", inner.putBlockBatchHits)
	}

	if _, _, err := store.PutBlock(ctx, []byte("hello"), nil); err != nil {
		t.Fatalf("PutBlock failed: %v", err)
	}
	if inner.putBlockHits != 1 {
		t.Fatalf("expected 1 PutBlock call, got %d", inner.putBlockHits)
	}

	if _, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{batchRef}); err != nil {
		t.Fatalf("GetBlockExistsBatch failed: %v", err)
	}
	if inner.existsBatchHits != 1 {
		t.Fatalf("expected 1 GetBlockExistsBatch call, got %d", inner.existsBatchHits)
	}
}

func TestBlockStoreReadOperationSharesDecodedBlockCache(t *testing.T) {
	ctx := context.Background()
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	store := &BlockStore{
		store:         newBatchForwardTestStore(),
		decodedBlocks: decodedBlocks,
	}
	scoped, release, err := store.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()

	scopedStore, ok := scoped.(*BlockStore)
	if !ok {
		t.Fatalf("scoped store type = %T, want *BlockStore", scoped)
	}
	if scopedStore.GetDecodedBlockCache() != decodedBlocks {
		t.Fatal("scoped read operation did not borrow block-store decoded cache")
	}
}

func TestBlockStoreRmBlockInvalidatesDecodedBlockCache(t *testing.T) {
	ctx := context.Background()
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	store := &BlockStore{
		store:         newBatchForwardTestStore(),
		decodedBlocks: decodedBlocks,
	}
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "removed"})
	if err != nil {
		t.Fatal(err.Error())
	}
	tx, cursor := block.NewTransaction(store, nil, ref, nil)
	tx.SetDecodedBlockCache(decodedBlocks)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()

	if err := store.RmBlock(ctx, ref); err != nil {
		t.Fatal(err.Error())
	}
	if data, found, err := store.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("GetBlock after RmBlock = len(%d), %v, %v; want missing block", len(data), found, err)
	}
	tx, cursor = block.NewTransaction(store, nil, ref, nil)
	tx.SetDecodedBlockCache(decodedBlocks)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("Unmarshal after RmBlock error = %v, want %v", err, block.ErrNotFound)
	}
}

func TestBlockStoreBatchTombstoneInvalidatesDecodedBlockCache(t *testing.T) {
	ctx := context.Background()
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		t.Fatal(err.Error())
	}
	defer decodedBlocks.Close()

	store := &BlockStore{
		store:         newBatchForwardTestStore(),
		decodedBlocks: decodedBlocks,
	}
	ref, _, err := block.PutBlock(ctx, store, &block_mock.Example{Msg: "removed"})
	if err != nil {
		t.Fatal(err.Error())
	}
	tx, cursor := block.NewTransaction(store, nil, ref, nil)
	tx.SetDecodedBlockCache(decodedBlocks)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); err != nil {
		t.Fatal(err.Error())
	}
	decodedBlocks.Wait()

	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}}); err != nil {
		t.Fatal(err.Error())
	}
	if data, found, err := store.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("GetBlock after tombstone batch = len(%d), %v, %v; want missing block", len(data), found, err)
	}
	tx, cursor = block.NewTransaction(store, nil, ref, nil)
	tx.SetDecodedBlockCache(decodedBlocks)
	if _, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("Unmarshal after tombstone batch error = %v, want %v", err, block.ErrNotFound)
	}
}
