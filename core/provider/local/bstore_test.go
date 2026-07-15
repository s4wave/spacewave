package provider_local

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_inmem "github.com/aperturerobotics/controllerbus/bus/inmem"
	bus_controller "github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	"github.com/s4wave/spacewave/db/dex"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
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

type localDEXTestController struct {
	value    []byte
	requests atomic.Int32
}

func (c *localDEXTestController) GetControllerInfo() *bus_controller.Info {
	return bus_controller.NewInfo(
		"test/local-dex",
		bus_controller.MustParseVersion("0.0.1"),
		"local DEX test controller",
	)
}

func (c *localDEXTestController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (c *localDEXTestController) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(dex.LookupBlockFromNetwork)
	if !ok {
		return nil, nil
	}
	return directive.R(directive.NewAccessResolver(func(
		_ context.Context,
		released func(),
	) (dex.LookupBlockFromNetworkValue, func(), error) {
		c.requests.Add(1)
		_ = dir
		return dex.NewLookupBlockFromNetworkValue(c.value, nil), func() {
			if released != nil {
				released()
			}
		}, nil
	}), nil)
}

func (c *localDEXTestController) Close() error { return nil }

var _ bus_controller.Controller = ((*localDEXTestController)(nil))

func newLocalDEXTestBus(
	ctx context.Context,
	value []byte,
) (bus.Bus, *localDEXTestController, func(), error) {
	dc := directive_controller.NewController(ctx, logrus.NewEntry(logrus.New()))
	b := bus_inmem.NewBus(dc)
	ctrl := &localDEXTestController{value: value}
	release, err := b.AddController(ctx, ctrl, nil)
	return b, ctrl, release, err
}

func TestLocalBlockStoreMissRoutesToSessionChildDEX(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ref, err := block.BuildBlockRef([]byte("from-session-dex"), nil)
	if err != nil {
		t.Fatal(err)
	}
	childBus, ctrl, release, err := newLocalDEXTestBus(ctx, []byte("from-session-dex"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	local := newBatchForwardTestStore()
	direct := &localDirectLookupStore{
		busForSession: func() bus.Bus { return childBus },
		bucketID:      "p/local/account/block-store",
		hashType:      ref.GetHash().GetHashType(),
	}
	store := &BlockStore{
		store:     block_store.NewStore("local-store", local),
		readStore: block_store.NewStore("local-store", &localReadStore{local: local, direct: direct}),
	}
	data, found, err := store.GetBlock(ctx, ref)
	if err != nil || !found || string(data) != "from-session-dex" {
		t.Fatalf("local DEX fallback = %q/%v/%v", data, found, err)
	}
	if ctrl.requests.Load() != 1 {
		t.Fatalf("local DEX requests = %d, want 1", ctrl.requests.Load())
	}
	if local.putBlockHits != 1 {
		t.Fatalf("local cache writes = %d, want 1", local.putBlockHits)
	}
	direct.busForSession = func() bus.Bus { return nil }
	data, found, err = store.GetBlock(ctx, ref)
	if err != nil || !found || string(data) != "from-session-dex" {
		t.Fatalf("local cache hit after child disable = %q/%v/%v", data, found, err)
	}
	if ctrl.requests.Load() != 1 {
		t.Fatalf("local DEX requests after cache hit = %d, want 1", ctrl.requests.Load())
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
