//go:build js

package blockshard

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

type cacheCoordinatorTestReader struct {
	mu     sync.Mutex
	data   []byte
	reads  int
	closes int
}

func (r *cacheCoordinatorTestReader) ReadAt(p []byte, off int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *cacheCoordinatorTestReader) Size() (int64, error) {
	return int64(len(r.data)), nil
}

func (r *cacheCoordinatorTestReader) Close() error {
	r.mu.Lock()
	r.closes++
	r.mu.Unlock()
	return nil
}

func (r *cacheCoordinatorTestReader) closeCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closes
}

func TestCacheCoordinatorChargesExactPayloadAndReleases(t *testing.T) {
	// Size the cache for one exact fixture payload.
	data, meta, key, expectedCharge := cacheCoordinatorFixture(t)
	cache := newCacheCoordinator(expectedCharge, 2)

	// Admit one segment with one metadata allocation and one file block.
	leaseA, readerA := acquireCoordinatorFixture(t, cache, 0, "a.sst", data, meta)
	assertCoordinatorValue(t, leaseA, key)
	leaseA.Release()
	stats := cache.snapshot()
	if stats.ChargedBytes != expectedCharge {
		t.Fatalf("charged bytes: got %d want %d", stats.ChargedBytes, expectedCharge)
	}
	if stats.LiveHandles != 1 {
		t.Fatalf("live handles: got %d want 1", stats.LiveHandles)
	}

	// Admit a second segment and evict the idle first segment within the limit.
	leaseB, readerB := acquireCoordinatorFixture(t, cache, 1, "b.sst", data, meta)
	assertCoordinatorValue(t, leaseB, key)
	leaseB.Release()
	stats = cache.snapshot()
	if stats.ChargedBytes > expectedCharge {
		t.Fatalf("charged bytes exceeded limit: got %d limit %d", stats.ChargedBytes, expectedCharge)
	}
	if stats.LiveHandles != 1 || stats.PeakLiveHandles > 2 {
		t.Fatalf("handle counts: live=%d peak=%d", stats.LiveHandles, stats.PeakLiveHandles)
	}
	if stats.Evictions == 0 {
		t.Fatal("expected capacity eviction")
	}
	if readerA.closeCount() != 1 {
		t.Fatalf("first reader closes: got %d want 1", readerA.closeCount())
	}

	// Close the coordinator and release the final admitted handle exactly once.
	cache.close()
	if readerB.closeCount() != 1 {
		t.Fatalf("second reader closes: got %d want 1", readerB.closeCount())
	}
	stats = cache.snapshot()
	if stats.ChargedBytes != 0 || stats.PinnedBytes != 0 || stats.LiveHandles != 0 || stats.ReleaseErrors != 0 {
		t.Fatalf("cache after close: charged=%d pinned=%d handles=%d release_errors=%d", stats.ChargedBytes, stats.PinnedBytes, stats.LiveHandles, stats.ReleaseErrors)
	}
}

func TestCacheCoordinatorHandleLimitEvictsIdleEntry(t *testing.T) {
	// Create one cache handle for two immutable fixture identities.
	data, meta, _, _ := cacheCoordinatorFixture(t)
	cache := newCacheCoordinator(^uint64(0), 1)

	// Fill the one-handle cache twice to force idle entry eviction.
	leaseA, readerA := acquireCoordinatorFixture(t, cache, 0, "a.sst", data, meta)
	leaseA.Release()
	leaseB, readerB := acquireCoordinatorFixture(t, cache, 1, "b.sst", data, meta)
	leaseB.Release()

	// Assert handle accounting and first-entry release.
	stats := cache.snapshot()
	if stats.LiveHandles != 1 || stats.PeakLiveHandles != 1 {
		t.Fatalf("handle limit: live=%d peak=%d", stats.LiveHandles, stats.PeakLiveHandles)
	}
	if readerA.closeCount() != 1 {
		t.Fatalf("evicted reader closes: got %d want 1", readerA.closeCount())
	}

	// Close the coordinator and release the remaining handle.
	cache.close()
	if readerB.closeCount() != 1 {
		t.Fatalf("resident reader closes: got %d want 1", readerB.closeCount())
	}
}

func TestCacheCoordinatorPinnedHandleBypassesAdmission(t *testing.T) {
	// Create one cache handle for competing lookup operations.
	data, meta, key, _ := cacheCoordinatorFixture(t)
	cache := newCacheCoordinator(^uint64(0), 1)

	// Keep the only admitted handle pinned through one lookup step.
	leaseA, readerA := acquireCoordinatorFixture(t, cache, 0, "a.sst", data, meta)
	assertCoordinatorValue(t, leaseA, key)

	// Complete the competing lookup with operation-local state.
	leaseB, readerB := acquireCoordinatorFixture(t, cache, 1, "b.sst", data, meta)
	assertCoordinatorValue(t, leaseB, key)
	leaseB.Release()
	stats := cache.snapshot()
	if stats.LiveHandles != 1 || stats.Bypasses == 0 {
		t.Fatalf("pinned bypass: handles=%d bypasses=%d", stats.LiveHandles, stats.Bypasses)
	}
	if readerB.closeCount() != 1 {
		t.Fatalf("bypassed reader closes: got %d want 1", readerB.closeCount())
	}
	if readerA.closeCount() != 0 {
		t.Fatalf("pinned reader closed early: got %d", readerA.closeCount())
	}

	// Release the admitted handle after the competing operation completes.
	leaseA.Release()
	cache.close()
	if readerA.closeCount() != 1 {
		t.Fatalf("admitted reader closes: got %d want 1", readerA.closeCount())
	}
}

func TestCacheCoordinatorPinnedRemovalDefersRelease(t *testing.T) {
	// Hold one lease on a segment that manifest refresh will retire.
	data, meta, key, _ := cacheCoordinatorFixture(t)
	cache := newCacheCoordinator(^uint64(0), 2)
	lease, reader := acquireCoordinatorFixture(t, cache, 7, "retired.sst", data, meta)

	// Retire the entry while its lookup lease remains active.
	cache.remove(cacheKey{shardID: 7, filename: "retired.sst"})
	if reader.closeCount() != 0 {
		t.Fatalf("pinned reader closed during removal: got %d", reader.closeCount())
	}
	assertCoordinatorValue(t, lease, key)
	lease.Release()
	if reader.closeCount() != 1 {
		t.Fatalf("retired reader closes after release: got %d want 1", reader.closeCount())
	}

	// Confirm deferred release drains every charge and handle.
	stats := cache.snapshot()
	if stats.ChargedBytes != 0 || stats.LiveHandles != 0 {
		t.Fatalf("retired cache state: charged=%d handles=%d", stats.ChargedBytes, stats.LiveHandles)
	}
}

func TestCacheCoordinatorOpenFailureRollsBackHandle(t *testing.T) {
	// Create one cache that can reserve a single file handle.
	_, meta, _, _ := cacheCoordinatorFixture(t)
	cache := newCacheCoordinator(^uint64(0), 1)

	// Fail file opening after the coordinator reserves one handle.
	_, err := cache.acquireSegment(
		context.Background(),
		cacheKey{shardID: 0, filename: "failure.sst"},
		meta,
		func() (segmentReader, error) {
			return nil, errors.New("open failed")
		},
	)
	if err == nil {
		t.Fatal("expected open failure")
	}

	// Confirm the failed admission rolls its reservation back.
	stats := cache.snapshot()
	if stats.LiveHandles != 0 || stats.ChargedBytes != 0 {
		t.Fatalf("failed admission state: handles=%d charged=%d", stats.LiveHandles, stats.ChargedBytes)
	}
}

func TestCacheCoordinatorPerSegmentBlockLimitBypassesPinnedPressure(t *testing.T) {
	// Build an immutable file spanning more blocks than one segment admits.
	data, meta := cacheCoordinatorLargeFixture(t)
	cache := newCacheCoordinator(^uint64(0), 2)
	lease, _ := acquireCoordinatorFixture(t, cache, 0, meta.Filename, data, meta)

	// Pin more aligned blocks than the shipping per-segment policy admits.
	for i := range maxCachedSegmentBlocks + 2 {
		var dst [1]byte
		if _, err := lease.ReadAt(dst[:], int64(i*cachedSegmentBlockSize)); err != nil {
			t.Fatal(err)
		}
	}
	stats := cache.snapshot()
	if len(lease.entry.blocks) > maxCachedSegmentBlocks {
		t.Fatalf("resident blocks: got %d limit %d", len(lease.entry.blocks), maxCachedSegmentBlocks)
	}
	if stats.Bypasses == 0 {
		t.Fatal("expected pinned block pressure to bypass admission")
	}

	// Release every retained resource after the pressure check.
	lease.Release()
	cache.close()
}

func TestCacheCoordinatorTouchesGlobalLRU(t *testing.T) {
	// Fill a two-handle cache with distinct immutable segments.
	data, meta, key, _ := cacheCoordinatorFixture(t)
	cache := newCacheCoordinator(^uint64(0), 2)
	leaseA, readerA := acquireCoordinatorFixture(t, cache, 0, "a.sst", data, meta)
	leaseA.Release()
	leaseB, readerB := acquireCoordinatorFixture(t, cache, 1, "b.sst", data, meta)
	leaseB.Release()

	// Touch both metadata and data for the first segment.
	leaseA, _ = acquireCoordinatorFixture(t, cache, 0, "a.sst", data, meta)
	assertCoordinatorValue(t, leaseA, key)
	leaseA.Release()

	// Admit a third handle and evict the untouched second segment.
	leaseC, readerC := acquireCoordinatorFixture(t, cache, 2, "c.sst", data, meta)
	leaseC.Release()
	if readerA.closeCount() != 0 {
		t.Fatalf("recent reader closed early: got %d", readerA.closeCount())
	}
	if readerB.closeCount() != 1 {
		t.Fatalf("least-recent reader closes: got %d want 1", readerB.closeCount())
	}

	// Close both remaining admitted handles.
	cache.close()
	if readerA.closeCount() != 1 || readerC.closeCount() != 1 {
		t.Fatalf("resident reader closes: a=%d c=%d", readerA.closeCount(), readerC.closeCount())
	}
}

func TestCacheCoordinatorOversizeReadBypassesAdmission(t *testing.T) {
	// Admit metadata and initial blocks for one large immutable segment.
	data, meta := cacheCoordinatorLargeFixture(t)
	cache := newCacheCoordinator(^uint64(0), 2)
	lease, _ := acquireCoordinatorFixture(t, cache, 0, meta.Filename, data, meta)
	before := cache.snapshot()

	// Read above the shipping threshold without changing retained charges.
	dst := make([]byte, maxCachedSegmentRead+1)
	n, err := lease.ReadAt(dst, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(dst) {
		t.Fatalf("oversize read bytes: got %d want %d", n, len(dst))
	}
	after := cache.snapshot()
	if after.ChargedBytes != before.ChargedBytes {
		t.Fatalf("oversize retained bytes: before=%d after=%d", before.ChargedBytes, after.ChargedBytes)
	}
	if after.Bypasses != before.Bypasses+1 || after.ReadCalls != before.ReadCalls+1 {
		t.Fatalf("oversize counters: bypasses=%d reads=%d", after.Bypasses-before.Bypasses, after.ReadCalls-before.ReadCalls)
	}

	// Release the admitted entry after the operation-local read.
	lease.Release()
	cache.close()
}

func cacheCoordinatorLargeFixture(t testing.TB) ([]byte, *SegmentMeta) {
	t.Helper()
	entries := make([]segment.Entry, 8)
	for i := range entries {
		entries[i] = segment.Entry{
			Key:   []byte{byte(i)},
			Value: bytes.Repeat([]byte{byte(i + 1)}, cachedSegmentBlockSize),
		}
	}
	data := buildSegmentData(t, entries)
	return data, &SegmentMeta{Filename: "large.sst", Size: uint32(len(data))}
}

func cacheCoordinatorFixture(t testing.TB) ([]byte, *SegmentMeta, []byte, uint64) {
	t.Helper()
	key := []byte("cache-key")
	data := buildSegmentData(t, []segment.Entry{{Key: key, Value: []byte("cache-value")}})
	lookup, err := segment.LoadLookupMeta(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	meta := &SegmentMeta{
		Filename:   "fixture.sst",
		EntryCount: 1,
		Size:       uint32(len(data)),
		MinKey:     key,
		MaxKey:     key,
	}
	return data, meta, key, lookupMetaCharge(lookup) + uint64(len(data))
}

func acquireCoordinatorFixture(
	t testing.TB,
	cache *cacheCoordinator,
	shardID int,
	filename string,
	data []byte,
	meta *SegmentMeta,
) (*segmentCacheLease, *cacheCoordinatorTestReader) {
	t.Helper()
	reader := &cacheCoordinatorTestReader{data: data}
	copyMeta := *meta
	copyMeta.Filename = filename
	lease, err := cache.acquireSegment(
		context.Background(),
		cacheKey{shardID: shardID, filename: filename},
		&copyMeta,
		func() (segmentReader, error) {
			return reader, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return lease, reader
}

func assertCoordinatorValue(t testing.TB, lease *segmentCacheLease, key []byte) {
	t.Helper()
	value, found, err := lease.lookup.Get(lease, key)
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(value) != "cache-value" {
		t.Fatalf("lookup returned found=%t value=%q", found, value)
	}
}
