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

// readCount reports snapshot reads without exposing the test reader mutex.
func (r *cacheCoordinatorTestReader) readCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reads
}

// cacheCoordinatorBlockingReader pauses the first snapshot read for fill-sharing tests.
type cacheCoordinatorBlockingReader struct {
	*cacheCoordinatorTestReader
	started  chan struct{}
	unblock  chan struct{}
	firstErr error
}

// ReadAt exposes the first in-flight read before returning scripted bytes or failure.
func (r *cacheCoordinatorBlockingReader) ReadAt(p []byte, off int64) (int, error) {
	r.mu.Lock()
	r.reads++
	call := r.reads
	r.mu.Unlock()

	// Hold the first call until a competing request joins its fill.
	if call == 1 {
		close(r.started)
		<-r.unblock
		if r.firstErr != nil {
			return 0, r.firstErr
		}
	}

	// Copy immutable fixture bytes with ReaderAt short-read semantics.
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// cacheCoordinatorReadResult carries one concurrent span request outcome.
type cacheCoordinatorReadResult struct {
	data []byte
	n    int
	err  error
}

// readCoordinatorSpan executes one lease read and publishes its complete outcome.
func readCoordinatorSpan(
	lease *segmentCacheLease,
	size int,
	results chan<- cacheCoordinatorReadResult,
) {
	data := make([]byte, size)
	n, err := lease.ReadAt(data, 0)
	results <- cacheCoordinatorReadResult{data: data, n: n, err: err}
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

func TestCacheCoordinatorPerSegmentSpanLimitBypassesPinnedPressure(t *testing.T) {
	// Build an immutable file spanning more fills than one segment admits.
	data, meta := cacheCoordinatorLargeFixture(t)
	cache := newCacheCoordinator(^uint64(0), 2)
	lease, _ := acquireCoordinatorFixture(t, cache, 0, meta.Filename, data, meta)

	// Pin more one-block spans than the shipping per-segment policy admits.
	for i := range maxCachedSegmentSpans + 2 {
		var dst [1]byte
		if _, err := lease.ReadAt(dst[:], int64(i*cachedSegmentBlockSize)); err != nil {
			t.Fatal(err)
		}
	}
	stats := cache.snapshot()
	if lease.entry.blockSpans > maxCachedSegmentSpans {
		t.Fatalf("resident spans: got %d limit %d", lease.entry.blockSpans, maxCachedSegmentSpans)
	}
	if len(lease.entry.blocks) > maxCachedSegmentSpans {
		t.Fatalf("resident block views: got %d limit %d", len(lease.entry.blocks), maxCachedSegmentSpans)
	}
	if stats.Bypasses == 0 {
		t.Fatal("expected pinned span pressure to bypass admission")
	}

	// Release every retained resource after the pressure check.
	lease.Release()
	cache.close()
}

func TestCacheCoordinatorChargesOneBackingAllocationPerSpan(t *testing.T) {
	// Read three consecutive missing blocks through one request-local allocation.
	data := bytes.Repeat([]byte("span"), cachedSegmentBlockSize*3/4)
	reader := &cacheCoordinatorTestReader{data: data}
	cache := newCacheCoordinator(^uint64(0), 1)
	lease, key := newCoordinatorDataLease(cache, reader, int64(len(data)))
	got := make([]byte, len(data))
	n, err := lease.ReadAt(got, 0)
	if err != nil || n != len(got) {
		t.Fatalf("span read: bytes=%d err=%v", n, err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("span read returned different bytes")
	}

	// Assert one read, one charge, one span, and three aligned block views.
	stats := cache.snapshot()
	wantCharge := uint64(len(data))
	if reader.readCount() != 1 || stats.ReadCalls != 1 || stats.FetchedBytes != wantCharge {
		t.Fatalf("snapshot reads: reader=%d calls=%d bytes=%d", reader.readCount(), stats.ReadCalls, stats.FetchedBytes)
	}
	if stats.ChargedBytes != wantCharge || stats.BlockBytes != wantCharge {
		t.Fatalf("span charge: charged=%d blocks=%d want=%d", stats.ChargedBytes, stats.BlockBytes, wantCharge)
	}
	if lease.entry.blockSpans != 1 || len(lease.entry.blocks) != 3 {
		t.Fatalf("resident span: spans=%d block_views=%d", lease.entry.blockSpans, len(lease.entry.blocks))
	}
	span := lease.entry.blocks[0].span
	for off := int64(0); off < int64(len(data)); off += cachedSegmentBlockSize {
		if block := lease.entry.blocks[off]; block == nil || block.span != span {
			t.Fatalf("block view at %d does not share the backing span", off)
		}
	}

	// Remove the idle span and its handle without leaving any block view or charge.
	lease.Release()
	cache.remove(key)
	stats = cache.snapshot()
	if len(lease.entry.blocks) != 0 || lease.entry.blockSpans != 0 {
		t.Fatalf("removed span: spans=%d block_views=%d", lease.entry.blockSpans, len(lease.entry.blocks))
	}
	if stats.ChargedBytes != 0 || stats.BlockBytes != 0 || stats.LiveHandles != 0 {
		t.Fatalf("removed charge: charged=%d blocks=%d handles=%d", stats.ChargedBytes, stats.BlockBytes, stats.LiveHandles)
	}
	if reader.closeCount() != 1 {
		t.Fatalf("removed reader closes: got %d want 1", reader.closeCount())
	}
}

func TestCacheCoordinatorSharesEqualInFlightSpan(t *testing.T) {
	// Pause one two-block snapshot read while an equal request starts.
	data := bytes.Repeat([]byte("fill"), cachedSegmentBlockSize*2/4)
	reader := &cacheCoordinatorBlockingReader{
		cacheCoordinatorTestReader: &cacheCoordinatorTestReader{data: data},
		started:                    make(chan struct{}),
		unblock:                    make(chan struct{}),
	}
	cache := newCacheCoordinator(^uint64(0), 1)
	leaseA, key := newCoordinatorDataLease(cache, reader, int64(len(data)))
	leaseB := newCoordinatorPeerLease(cache, leaseA.entry)
	results := make(chan cacheCoordinatorReadResult, 2)
	go readCoordinatorSpan(leaseA, len(data), results)
	<-reader.started
	go readCoordinatorSpan(leaseB, len(data), results)

	// Wait until the equal request joins, then release the single underlying read.
	cache.mu.Lock()
	for cache.stats.SharedFills != 1 {
		cache.cond.Wait()
	}
	cache.mu.Unlock()
	close(reader.unblock)
	first := <-results
	second := <-results

	// Require identical successful bytes from one snapshot call and one admission.
	for i, result := range []cacheCoordinatorReadResult{first, second} {
		if result.err != nil || result.n != len(data) {
			t.Fatalf("result %d: bytes=%d err=%v", i, result.n, result.err)
		}
		if !bytes.Equal(result.data, data) {
			t.Fatalf("result %d returned different bytes", i)
		}
	}
	stats := cache.snapshot()
	if reader.readCount() != 1 || stats.ReadCalls != 1 || stats.SharedFills != 1 || stats.Admissions != 1 {
		t.Fatalf("shared fill: reader=%d calls=%d shared=%d admissions=%d", reader.readCount(), stats.ReadCalls, stats.SharedFills, stats.Admissions)
	}

	// Release both pins and retire the shared span.
	leaseA.Release()
	leaseB.Release()
	cache.remove(key)
}

func TestCacheCoordinatorSharesFailureAndAllowsRetry(t *testing.T) {
	// Pause one failing snapshot read while an equal request starts.
	fillErr := errors.New("scripted fill failure")
	data := bytes.Repeat([]byte("retry"), cachedSegmentBlockSize/5+1)[:cachedSegmentBlockSize]
	reader := &cacheCoordinatorBlockingReader{
		cacheCoordinatorTestReader: &cacheCoordinatorTestReader{data: data},
		started:                    make(chan struct{}),
		unblock:                    make(chan struct{}),
		firstErr:                   fillErr,
	}
	cache := newCacheCoordinator(^uint64(0), 1)
	leaseA, key := newCoordinatorDataLease(cache, reader, int64(len(data)))
	leaseB := newCoordinatorPeerLease(cache, leaseA.entry)
	results := make(chan cacheCoordinatorReadResult, 2)
	go readCoordinatorSpan(leaseA, len(data), results)
	<-reader.started
	go readCoordinatorSpan(leaseB, len(data), results)

	// Wait for fill sharing before publishing the leader failure.
	cache.mu.Lock()
	for cache.stats.SharedFills != 1 {
		cache.cond.Wait()
	}
	cache.mu.Unlock()
	close(reader.unblock)
	first := <-results
	second := <-results

	// Require the same failure and no resident state from either request.
	for i, result := range []cacheCoordinatorReadResult{first, second} {
		if result.n != 0 || result.err != fillErr {
			t.Fatalf("failure %d: bytes=%d err=%v", i, result.n, result.err)
		}
	}
	stats := cache.snapshot()
	if stats.ChargedBytes != 0 || stats.BlockBytes != 0 || stats.Admissions != 0 {
		t.Fatalf("failed fill residency: charged=%d blocks=%d admissions=%d", stats.ChargedBytes, stats.BlockBytes, stats.Admissions)
	}
	if len(leaseA.entry.fills) != 0 || len(leaseA.entry.blocks) != 0 || leaseA.entry.blockSpans != 0 {
		t.Fatalf("failed fill state: fills=%d block_views=%d spans=%d", len(leaseA.entry.fills), len(leaseA.entry.blocks), leaseA.entry.blockSpans)
	}

	// Start a new lease after failure and admit its successful retry.
	retryLease := newCoordinatorPeerLease(cache, leaseA.entry)
	leaseA.Release()
	leaseB.Release()
	got := make([]byte, len(data))
	n, err := retryLease.ReadAt(got, 0)
	if err != nil || n != len(got) {
		t.Fatalf("retry: bytes=%d err=%v", n, err)
	}
	if !bytes.Equal(got, data) || reader.readCount() != 2 {
		t.Fatalf("retry bytes or calls: equal=%t calls=%d", bytes.Equal(got, data), reader.readCount())
	}
	stats = cache.snapshot()
	if stats.Admissions != 1 || stats.ReadCalls != 2 {
		t.Fatalf("retry counters: admissions=%d calls=%d", stats.Admissions, stats.ReadCalls)
	}

	// Release and retire the successfully retried span.
	retryLease.Release()
	cache.remove(key)
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

// newCoordinatorDataLease constructs one admitted data-only entry for focused span tests.
func newCoordinatorDataLease(
	cache *cacheCoordinator,
	reader segmentReader,
	size int64,
) (*segmentCacheLease, cacheKey) {
	key := cacheKey{shardID: 0, filename: "data-only.sst"}
	entry := &cacheEntry{
		key:    key,
		blocks: make(map[int64]*cachedBlock),
		fills:  make(map[segmentFillRange]*segmentFill),
		leases: 1,
	}
	entry.file = newCoordinatedSegmentFile(reader, size, cache, entry)
	cache.entries[key] = entry
	cache.stats.LiveHandles = 1
	cache.stats.PeakLiveHandles = 1
	return &segmentCacheLease{
		cache:     cache,
		entry:     entry,
		resources: make(map[*cacheResource]struct{}),
	}, key
}

// newCoordinatorPeerLease creates another operation lease for one admitted entry.
func newCoordinatorPeerLease(cache *cacheCoordinator, entry *cacheEntry) *segmentCacheLease {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return cache.newLeaseLocked(entry)
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
