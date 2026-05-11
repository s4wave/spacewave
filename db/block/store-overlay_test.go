package block

import (
	"context"
	"sync"
	"testing"
	"time"

	hash "github.com/s4wave/spacewave/net/hash"
)

type overlayBatchTestStore struct {
	NopStoreOps

	mu               sync.Mutex
	putCalls         int
	rmCalls          int
	batchCalls       int
	backgroundCalls  int
	existsBatchCalls int
	getData          []byte
}

type overlayNilOptsTestStore struct {
	NopStoreOps

	putOpts        []*PutOpts
	backgroundOpts []*PutOpts
}

func (s *overlayBatchTestStore) GetHashType() hash.HashType {
	return hash.HashType_HashType_BLAKE3
}

func (s *overlayBatchTestStore) PutBlock(_ context.Context, _ []byte, opts *PutOpts) (*BlockRef, bool, error) {
	s.mu.Lock()
	s.putCalls++
	s.mu.Unlock()
	return opts.GetForceBlockRef(), false, nil
}

func (s *overlayBatchTestStore) GetBlock(context.Context, *BlockRef) ([]byte, bool, error) {
	if s.getData != nil {
		return s.getData, true, nil
	}
	return nil, false, nil
}

func (s *overlayBatchTestStore) GetBlockExists(context.Context, *BlockRef) (bool, error) {
	return false, nil
}

func (s *overlayBatchTestStore) StatBlock(context.Context, *BlockRef) (*BlockStat, error) {
	return nil, nil
}

func (s *overlayBatchTestStore) RmBlock(context.Context, *BlockRef) error {
	s.mu.Lock()
	s.rmCalls++
	s.mu.Unlock()
	return nil
}

func (s *overlayBatchTestStore) PutBlockBatch(_ context.Context, _ []*PutBatchEntry) error {
	s.batchCalls++
	return nil
}

func (s *overlayBatchTestStore) PutBlockBackground(_ context.Context, _ []byte, opts *PutOpts) (*BlockRef, bool, error) {
	s.backgroundCalls++
	return opts.GetForceBlockRef(), false, nil
}

func (s *overlayBatchTestStore) GetBlockExistsBatch(_ context.Context, refs []*BlockRef) ([]bool, error) {
	s.existsBatchCalls++
	return make([]bool, len(refs)), nil
}

func (s *overlayNilOptsTestStore) GetHashType() hash.HashType {
	return hash.HashType_HashType_BLAKE3
}

func (s *overlayNilOptsTestStore) PutBlock(_ context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	s.putOpts = append(s.putOpts, opts)
	ref, err := BuildBlockRef(data, opts)
	return ref, false, err
}

func (s *overlayNilOptsTestStore) PutBlockBackground(_ context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	s.backgroundOpts = append(s.backgroundOpts, opts)
	ref, err := BuildBlockRef(data, opts)
	return ref, false, err
}

func TestStoreOverlayPutBlockBatchForwards(t *testing.T) {
	ctx := context.Background()
	lower := &overlayBatchTestStore{}
	upper := &overlayBatchTestStore{}
	overlay := NewOverlay(ctx, nil, lower, upper, OverlayMode_UPPER_CACHE, 0, nil)
	ref := &BlockRef{Hash: hash.NewHash(hash.HashType_HashType_BLAKE3, []byte{1})}
	entries := []*PutBatchEntry{{Ref: ref, Data: []byte("hello")}}

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
	lower := &overlayNilOptsTestStore{}
	upper := &overlayNilOptsTestStore{}
	overlay := NewOverlay(ctx, nil, lower, upper, OverlayMode_UPPER_CACHE, 0, nil)

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
	lower := &overlayBatchTestStore{}
	upper := &overlayBatchTestStore{}
	overlay := NewOverlay(ctx, nil, lower, upper, OverlayMode_UPPER_ONLY, 0, nil)
	ref := &BlockRef{Hash: hash.NewHash(hash.HashType_HashType_BLAKE3, []byte{2})}

	if _, _, err := overlay.PutBlockBackground(ctx, []byte("hello"), &PutOpts{ForceBlockRef: ref}); err != nil {
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
	lower := &overlayNilOptsTestStore{}
	upper := &overlayNilOptsTestStore{}
	overlay := NewOverlay(ctx, nil, lower, upper, OverlayMode_UPPER_CACHE, 0, nil)

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
	lower := &overlayBatchTestStore{getData: []byte("from-lower")}
	upper := &overlayBatchTestStore{}
	overlay := NewOverlay(ctx, nil, lower, upper, OverlayMode_UPPER_READBACK_CACHE, 0, nil)
	ref := &BlockRef{Hash: hash.NewHash(hash.HashType_HashType_BLAKE3, []byte{4})}

	data, found, err := overlay.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found || string(data) != "from-lower" {
		t.Fatalf("expected lower data, got found=%v data=%q", found, string(data))
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		upper.mu.Lock()
		n := upper.putCalls
		upper.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
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

	if _, _, err := overlay.PutBlock(ctx, []byte("hello"), &PutOpts{ForceBlockRef: ref}); err != nil {
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
	ref := &BlockRef{Hash: hash.NewHash(hash.HashType_HashType_BLAKE3, []byte{5})}

	for _, tt := range []struct {
		name      string
		mode      OverlayMode
		wantLower int
		wantUpper int
	}{
		{name: "upper", mode: OverlayMode_UPPER_WRITE_CACHE, wantUpper: 1},
		{name: "lower", mode: OverlayMode_LOWER_WRITE_CACHE, wantLower: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lower := &overlayBatchTestStore{}
			upper := &overlayBatchTestStore{}
			overlay := NewOverlay(ctx, nil, lower, upper, tt.mode, 0, nil)

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
	lower := &overlayBatchTestStore{}
	upper := &overlayBatchTestStore{}
	overlay := NewOverlay(ctx, nil, lower, upper, OverlayMode_UPPER_READ_CACHE, 0, nil)
	ref := &BlockRef{Hash: hash.NewHash(hash.HashType_HashType_BLAKE3, []byte{3})}

	if _, err := overlay.GetBlockExistsBatch(ctx, []*BlockRef{ref}); err != nil {
		t.Fatal(err.Error())
	}
	if upper.existsBatchCalls != 1 {
		t.Fatalf("expected upper batch exists call, got %d", upper.existsBatchCalls)
	}
	if lower.existsBatchCalls != 1 {
		t.Fatalf("expected lower batch exists call for cache miss fallback, got %d", lower.existsBatchCalls)
	}
}
