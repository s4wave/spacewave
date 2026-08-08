package kvtx

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/coord/conformance"
	db_kvtx "github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

type countingBatchStore struct {
	block.StoreOps

	putCalls         int
	putBatchCalls    int
	existsBatchCalls int
}

func newCountingBatchStore() *countingBatchStore {
	return &countingBatchStore{
		StoreOps: block_store_inmem.NewInmemBlock(
			store_kvkey.NewDefaultKVKey(),
			store_kvtx_inmem.NewStore(),
			0,
			false,
		),
	}
}

func (s *countingBatchStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.putCalls++
	return s.StoreOps.PutBlock(ctx, data, opts)
}

func (s *countingBatchStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	s.putBatchCalls++
	return s.StoreOps.PutBlockBatch(ctx, entries)
}

func (s *countingBatchStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchCalls++
	return s.StoreOps.GetBlockExistsBatch(ctx, refs)
}

var _ block.StoreOps = (*countingBatchStore)(nil)

type countingTxStore struct {
	db_kvtx.Store

	newTxCalls int
}

func (s *countingTxStore) NewTransaction(ctx context.Context, write bool) (db_kvtx.Tx, error) {
	s.newTxCalls++
	return s.Store.NewTransaction(ctx, write)
}

var _ db_kvtx.Store = (*countingTxStore)(nil)

func TestVolumeForwardsBatchPut(t *testing.T) {
	ctx := context.Background()
	kvKey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatalf("NewKVKey failed: %v", err)
	}

	inner := newCountingBatchStore()
	vol, err := NewVolumeWithBlockStore(
		ctx,
		"hydra/test-volume",
		kvKey,
		store_kvtx_inmem.NewStore(),
		inner,
		&store_kvtx.Config{},
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewVolumeWithBlockStore failed: %v", err)
	}
	t.Cleanup(func() { _ = vol.Close() })

	ref1, err := block.BuildBlockRef([]byte("hello"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}
	ref2, err := block.BuildBlockRef([]byte("world"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef failed: %v", err)
	}

	if err := vol.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: ref1, Data: []byte("hello")},
		{Ref: ref2, Data: []byte("world")},
	}); err != nil {
		t.Fatalf("PutBlockBatch failed: %v", err)
	}

	if inner.putBatchCalls != 1 {
		t.Fatalf("expected 1 PutBlockBatch call, got %d", inner.putBatchCalls)
	}
	if inner.putCalls != 0 {
		t.Fatalf("expected 0 fallback PutBlock calls, got %d", inner.putCalls)
	}

	if _, err := vol.GetBlockExistsBatch(ctx, []*block.BlockRef{ref1, ref2}); err != nil {
		t.Fatalf("GetBlockExistsBatch failed: %v", err)
	}
	if inner.existsBatchCalls != 1 {
		t.Fatalf("expected 1 GetBlockExistsBatch call, got %d", inner.existsBatchCalls)
	}
}

func TestVolumeWithBlockStoreInitializesKeyedCoordinator(t *testing.T) {
	ctx := context.Background()
	vol, err := NewVolumeWithBlockStore(
		ctx,
		"hydra/test-volume",
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		newCountingBatchStore(),
		&store_kvtx.Config{},
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewVolumeWithBlockStore failed: %v", err)
	}
	t.Cleanup(func() { _ = vol.Close() })

	lease, acquired, err := vol.TryAcquireWriteLease(ctx, coord.Scope{VolumeID: vol.GetID(), Key: "object-a"})
	if err != nil || !acquired {
		t.Fatalf("TryAcquireWriteLease failed: acquired=%v err=%v", acquired, err)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatalf("WriteLease.Release failed: %v", err)
	}
}

func TestVolumeWithBlockStoreAndGCInitializesKeyedCoordinator(t *testing.T) {
	ctx := context.Background()
	vol, err := NewVolumeWithBlockStoreAndGC(
		ctx,
		"hydra/test-volume",
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		newCountingBatchStore(),
		nil,
		&store_kvtx.Config{},
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewVolumeWithBlockStoreAndGC failed: %v", err)
	}
	t.Cleanup(func() { _ = vol.Close() })

	lease, acquired, err := vol.TryAcquireWriteLease(ctx, coord.Scope{VolumeID: vol.GetID(), Key: "object-a"})
	if err != nil || !acquired {
		t.Fatalf("TryAcquireWriteLease failed: acquired=%v err=%v", acquired, err)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatalf("WriteLease.Release failed: %v", err)
	}
}

func TestVolumeEmbedsInMemoryCoordinator(t *testing.T) {
	ctx := context.Background()
	vol, err := NewVolume(
		ctx,
		"hydra/test-volume",
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		&store_kvtx.Config{},
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewVolume failed: %v", err)
	}
	t.Cleanup(func() { _ = vol.Close() })

	capability, err := vol.Capability(ctx, coord.Scope{
		VolumeID:      vol.GetID(),
		ObjectStoreID: "objects",
		ParticipantID: "process-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !capability.Supported {
		t.Fatal("expected in-memory coordinator capability to be supported")
	}
	if capability.Backend != coord.BackendKindInMemory {
		t.Fatalf("unexpected backend: %q", capability.Backend)
	}
	if capability.VolumeID != vol.GetID() || capability.ObjectStoreID != "objects" {
		t.Fatalf("unexpected capability scope: %#v", capability)
	}
}

func TestVolumeCoordinatorConformance(t *testing.T) {
	conformance.Check(t, func(tb testing.TB) (coord.Coordinator, coord.Coordinator) {
		store := store_kvtx_inmem.NewStore()
		kvkey := store_kvkey.NewDefaultKVKey()
		volA, err := NewVolume(
			context.Background(),
			"hydra/test-volume",
			kvkey,
			store,
			&store_kvtx.Config{},
			false,
			false,
			nil,
			nil,
		)
		if err != nil {
			tb.Fatalf("NewVolume failed: %v", err)
		}
		volB, err := NewVolume(
			context.Background(),
			"hydra/test-volume",
			kvkey,
			store,
			&store_kvtx.Config{},
			false,
			false,
			nil,
			nil,
		)
		if err != nil {
			tb.Fatalf("second NewVolume failed: %v", err)
		}
		tb.Cleanup(func() {
			_ = volB.Close()
			_ = volA.Close()
		})
		if volA.GetID() != volB.GetID() {
			tb.Fatalf("independent handles have different volume ids: %q != %q", volA.GetID(), volB.GetID())
		}
		return volA, volB
	})
}

func TestCoordinatorLeaseDoesNotOpenBackendTransaction(t *testing.T) {
	ctx := context.Background()
	store := &countingTxStore{Store: store_kvtx_inmem.NewStore()}
	vol, err := NewVolume(
		ctx,
		"hydra/test-volume",
		store_kvkey.NewDefaultKVKey(),
		store,
		&store_kvtx.Config{},
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewVolume failed: %v", err)
	}
	t.Cleanup(func() { _ = vol.Close() })

	store.newTxCalls = 0
	scope := coord.Scope{
		VolumeID:      vol.GetID(),
		ObjectStoreID: "objects",
		ParticipantID: "process-a",
	}

	if _, err := vol.Capability(ctx, scope); err != nil {
		t.Fatal(err)
	}
	if _, err := vol.Snapshot(ctx, scope); err != nil {
		t.Fatal(err)
	}
	watch, err := vol.Watch(ctx, scope, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	lease, ok, err := vol.TryAcquireWriteLease(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("lease unexpectedly busy")
	}
	if _, err := lease.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := lease.Publish(ctx, coord.Event{KeyPrefixChanged: []byte("world-head/")}); err != nil {
		t.Fatal(err)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if store.newTxCalls != 0 {
		t.Fatalf("coordinator opened %d backend transactions", store.newTxCalls)
	}

	tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	tx.Discard()
	if store.newTxCalls != 1 {
		t.Fatalf("volume NewTransaction opened %d backend transactions, want 1", store.newTxCalls)
	}
}
