package block_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	hash "github.com/s4wave/spacewave/net/hash"
)

type overlayBatchTestStore struct {
	block.StoreOps

	mu               sync.Mutex
	putCalls         int
	rmCalls          int
	batchCalls       int
	backgroundCalls  int
	existsBatchCalls int
	putNotify        chan struct{}
}

type overlayNilOptsTestStore struct {
	block.StoreOps

	putOpts        []*block.PutOpts
	backgroundOpts []*block.PutOpts
}

func newOverlayBatchTestStore() *overlayBatchTestStore {
	return &overlayBatchTestStore{
		StoreOps:  newOverlayMemoryStore(),
		putNotify: make(chan struct{}, 1),
	}
}

func newOverlayNilOptsTestStore() *overlayNilOptsTestStore {
	return &overlayNilOptsTestStore{StoreOps: newOverlayMemoryStore()}
}

func newOverlayMemoryStore() block.StoreOps {
	return block_store_inmem.NewInmemBlock(
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		hash.HashType_HashType_BLAKE3,
		false,
	)
}

func (s *overlayBatchTestStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.mu.Lock()
	s.putCalls++
	s.mu.Unlock()
	select {
	case s.putNotify <- struct{}{}:
	default:
	}
	return s.StoreOps.PutBlock(ctx, data, opts)
}

func (s *overlayBatchTestStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	s.mu.Lock()
	s.rmCalls++
	s.mu.Unlock()
	return s.StoreOps.RmBlock(ctx, ref)
}

func (s *overlayBatchTestStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	s.batchCalls++
	return s.StoreOps.PutBlockBatch(ctx, entries)
}

func (s *overlayBatchTestStore) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.backgroundCalls++
	return s.StoreOps.PutBlockBackground(ctx, data, opts)
}

func (s *overlayBatchTestStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.existsBatchCalls++
	return s.StoreOps.GetBlockExistsBatch(ctx, refs)
}

func (s *overlayNilOptsTestStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.putOpts = append(s.putOpts, opts)
	return s.StoreOps.PutBlock(ctx, data, opts)
}

func (s *overlayNilOptsTestStore) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	s.backgroundOpts = append(s.backgroundOpts, opts)
	return s.StoreOps.PutBlockBackground(ctx, data, opts)
}

func TestStoreOverlayPutBlockBatchForwards(t *testing.T) {
	ctx := context.Background()
	lower := newOverlayBatchTestStore()
	upper := newOverlayBatchTestStore()
	overlay := block.NewOverlay(ctx, nil, lower, upper, block.OverlayMode_UPPER_CACHE, 0, nil)
	data := []byte("hello")
	ref := mustBuildBlockRef(t, data)
	entries := []*block.PutBatchEntry{{Ref: ref, Data: data}}

	if err := overlay.PutBlockBatch(ctx, entries); err != nil {
		t.Fatal(err.Error())
	}

	if lower.batchCalls != 1 || upper.batchCalls != 1 {
		t.Fatalf("expected both stores to receive one batch call, got lower=%d upper=%d", lower.batchCalls, upper.batchCalls)
	}
	if lower.putCalls != 0 || upper.putCalls != 0 {
		t.Fatalf("expected no per-entry PutBlock fallback, got lower=%d upper=%d", lower.putCalls, upper.putCalls)
	}
}

func TestStoreOverlayCachePutHandlesNilOpts(t *testing.T) {
	ctx := context.Background()
	lower := newOverlayNilOptsTestStore()
	upper := newOverlayNilOptsTestStore()
	overlay := block.NewOverlay(ctx, nil, lower, upper, block.OverlayMode_UPPER_CACHE, 0, nil)

	ref, _, err := overlay.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(lower.putOpts) != 1 || lower.putOpts[0] != nil {
		t.Fatalf("expected primary put to receive original nil opts")
	}
	if len(upper.putOpts) != 1 {
		t.Fatalf("expected secondary put to receive forced-ref opts")
	}
	if !upper.putOpts[0].GetForceBlockRef().EqualsRef(ref) {
		t.Fatalf("expected secondary put force ref to match primary ref")
	}
}

func TestStoreOverlayPutBlockBackgroundForwards(t *testing.T) {
	ctx := context.Background()
	lower := newOverlayBatchTestStore()
	upper := newOverlayBatchTestStore()
	overlay := block.NewOverlay(ctx, nil, lower, upper, block.OverlayMode_UPPER_ONLY, 0, nil)
	data := []byte("hello")
	ref := mustBuildBlockRef(t, data)

	if _, _, err := overlay.PutBlockBackground(ctx, data, &block.PutOpts{ForceBlockRef: ref}); err != nil {
		t.Fatal(err.Error())
	}

	if upper.backgroundCalls != 1 {
		t.Fatalf("expected upper background call, got %d", upper.backgroundCalls)
	}
	if upper.putCalls != 0 {
		t.Fatalf("expected no foreground fallback PutBlock calls, got %d", upper.putCalls)
	}
}

func TestStoreOverlayCacheBackgroundPutHandlesNilOpts(t *testing.T) {
	ctx := context.Background()
	lower := newOverlayNilOptsTestStore()
	upper := newOverlayNilOptsTestStore()
	overlay := block.NewOverlay(ctx, nil, lower, upper, block.OverlayMode_UPPER_CACHE, 0, nil)

	ref, _, err := overlay.PutBlockBackground(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(lower.backgroundOpts) != 1 || lower.backgroundOpts[0] != nil {
		t.Fatalf("expected primary background put to receive original nil opts")
	}
	if len(upper.backgroundOpts) != 1 {
		t.Fatalf("expected secondary background put to receive forced-ref opts")
	}
	if !upper.backgroundOpts[0].GetForceBlockRef().EqualsRef(ref) {
		t.Fatalf("expected secondary background put force ref to match primary ref")
	}
}

func TestStoreOverlayUpperReadbackCache(t *testing.T) {
	ctx := context.Background()
	lower := newOverlayBatchTestStore()
	upper := newOverlayBatchTestStore()
	overlay := block.NewOverlay(ctx, nil, lower, upper, block.OverlayMode_UPPER_READBACK_CACHE, 0, nil)
	data := []byte("from-lower")
	ref, _, err := lower.StoreOps.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	data, found, err := overlay.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found || string(data) != "from-lower" {
		t.Fatalf("expected lower data, got found=%v data=%q", found, string(data))
	}

	select {
	case <-upper.putNotify:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for writeback to upper")
	}
	upper.mu.Lock()
	upperPuts := upper.putCalls
	upper.mu.Unlock()
	lower.mu.Lock()
	lowerPuts := lower.putCalls
	lower.mu.Unlock()
	if upperPuts != 1 {
		t.Fatalf("expected one writeback to upper, got %d", upperPuts)
	}
	if lowerPuts != 0 {
		t.Fatalf("expected no writes to lower, got %d", lowerPuts)
	}

	if _, _, err := overlay.PutBlock(ctx, data, &block.PutOpts{ForceBlockRef: ref}); err != nil {
		t.Fatal(err.Error())
	}
	upper.mu.Lock()
	upperPuts = upper.putCalls
	upper.mu.Unlock()
	lower.mu.Lock()
	lowerPuts = lower.putCalls
	lower.mu.Unlock()
	if upperPuts != 2 {
		t.Fatalf("expected upper put to bring total to 2, got %d", upperPuts)
	}
	if lowerPuts != 0 {
		t.Fatalf("expected no writes to lower, got %d", lowerPuts)
	}

	if err := overlay.RmBlock(ctx, ref); err != nil {
		t.Fatal(err.Error())
	}
	upper.mu.Lock()
	upperRms := upper.rmCalls
	upper.mu.Unlock()
	lower.mu.Lock()
	lowerRms := lower.rmCalls
	lower.mu.Unlock()
	if upperRms != 1 {
		t.Fatalf("expected one rm on upper, got %d", upperRms)
	}
	if lowerRms != 0 {
		t.Fatalf("expected no rms on lower, got %d", lowerRms)
	}
}

func TestStoreOverlayWriteCacheRemoveUsesWriteStore(t *testing.T) {
	ctx := context.Background()
	ref := mustBuildBlockRef(t, []byte("remove"))

	for _, tt := range []struct {
		name      string
		mode      block.OverlayMode
		wantLower int
		wantUpper int
	}{
		{name: "upper", mode: block.OverlayMode_UPPER_WRITE_CACHE, wantUpper: 1},
		{name: "lower", mode: block.OverlayMode_LOWER_WRITE_CACHE, wantLower: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lower := newOverlayBatchTestStore()
			upper := newOverlayBatchTestStore()
			overlay := block.NewOverlay(ctx, nil, lower, upper, tt.mode, 0, nil)

			if err := overlay.RmBlock(ctx, ref); err != nil {
				t.Fatal(err.Error())
			}
			if lower.rmCalls != tt.wantLower || upper.rmCalls != tt.wantUpper {
				t.Fatalf("unexpected rm calls: lower=%d upper=%d", lower.rmCalls, upper.rmCalls)
			}
		})
	}
}

func TestStoreOverlayGetBlockExistsBatchForwards(t *testing.T) {
	ctx := context.Background()
	lower := newOverlayBatchTestStore()
	upper := newOverlayBatchTestStore()
	overlay := block.NewOverlay(ctx, nil, lower, upper, block.OverlayMode_UPPER_READ_CACHE, 0, nil)
	ref := mustBuildBlockRef(t, []byte("missing"))

	if _, err := overlay.GetBlockExistsBatch(ctx, []*block.BlockRef{ref}); err != nil {
		t.Fatal(err.Error())
	}
	if upper.existsBatchCalls != 1 {
		t.Fatalf("expected upper batch exists call, got %d", upper.existsBatchCalls)
	}
	if lower.existsBatchCalls != 1 {
		t.Fatalf("expected lower batch exists call for cache miss fallback, got %d", lower.existsBatchCalls)
	}
}

func mustBuildBlockRef(t *testing.T, data []byte) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef(data, &block.PutOpts{HashType: hash.HashType_HashType_BLAKE3})
	if err != nil {
		t.Fatal(err.Error())
	}
	return ref
}
