//go:build js

package blockshard

import (
	"context"
	"encoding/binary"
	"os"
	"runtime"
	"strconv"
	"syscall/js"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

const (
	cacheScaleSegmentsEnv    = "SPACEWAVE_OPFS_CACHE_SCALE_SEGMENTS"
	cacheScaleEntriesEnv     = "SPACEWAVE_OPFS_CACHE_SCALE_ENTRIES"
	defaultCacheScaleEntries = 4096
)

func TestCachedSegmentFileScale(t *testing.T) {
	// Select one isolated geometric corpus size and fixture width.
	countText := os.Getenv(cacheScaleSegmentsEnv)
	if countText == "" {
		t.Skipf("set %s to the active segment count", cacheScaleSegmentsEnv)
	}
	count, err := strconv.Atoi(countText)
	if err != nil || count < 1 {
		t.Fatalf("%s=%q is not a positive integer", cacheScaleSegmentsEnv, countText)
	}
	entryCount := defaultCacheScaleEntries
	if entriesText := os.Getenv(cacheScaleEntriesEnv); entriesText != "" {
		entryCount, err = strconv.Atoi(entriesText)
		if err != nil || entryCount < 1 {
			t.Fatalf("%s=%q is not a positive integer", cacheScaleEntriesEnv, entriesText)
		}
	}

	// Build one deterministic SSTable shared by every cache identity.
	entries := make([]segment.Entry, entryCount)
	value := make([]byte, 176)
	for i := range entries {
		key := make([]byte, 4)
		binary.BigEndian.PutUint32(key, uint32(i))
		entries[i] = segment.Entry{Key: key, Value: value}
	}
	data := buildSegmentData(t, entries)

	// Write the fixture through real browser OPFS.
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	const dirName = "blockshard-cache-scale"
	_ = opfs.DeleteEntry(root, dirName, true)
	dir, err := opfs.GetDirectory(root, dirName, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := opfs.DeleteEntry(root, dirName, true); err != nil {
			t.Error(err)
		}
	})
	const filename = "segment.sst"
	if err := opfs.WriteFile(dir, filename, data); err != nil {
		t.Fatal(err)
	}
	meta := &SegmentMeta{
		Filename:   filename,
		EntryCount: uint32(len(entries)),
		Size:       uint32(len(data)),
		MinKey:     entries[0].Key,
		MaxKey:     entries[len(entries)-1].Key,
	}

	// Capture the runtime baseline before retaining coordinated resources.
	runtime.GC()
	var goBefore runtime.MemStats
	runtime.ReadMemStats(&goBefore)
	jsBefore, jsStatus := cacheScaleJSHeap()
	started := time.Now()
	cache := newDefaultCacheCoordinator()
	t.Cleanup(cache.close)

	// Touch one independent segment identity and release each lookup pin.
	for i := range count {
		lease, err := cache.acquireSegment(
			context.Background(),
			cacheKey{shardID: i, filename: filename},
			meta,
			func() (segmentReader, error) {
				return opfs.OpenReadSnapshot(dir, filename)
			},
		)
		if err != nil {
			t.Fatalf("acquire segment %d: %v", i, err)
		}
		key := entries[(i*2053)%len(entries)].Key
		got, found, err := lease.lookup.Get(lease, key)
		lease.Release()
		if err != nil {
			t.Fatalf("read segment %d: %v", i, err)
		}
		if !found || len(got) != len(value) {
			t.Fatalf("read segment %d returned found=%t bytes=%d", i, found, len(got))
		}
	}

	// Capture heaps while the coordinator retains its bounded working set.
	runtime.GC()
	var goAfter runtime.MemStats
	runtime.ReadMemStats(&goAfter)
	jsAfter, afterStatus := cacheScaleJSHeap()
	if afterStatus != jsStatus {
		jsStatus += "-then-" + afterStatus
	}
	stats := cache.snapshot()

	// Report one parseable row for the geometric evidence table.
	jsDelta := int64(0)
	if jsStatus == "available" {
		jsDelta = int64(jsAfter) - int64(jsBefore)
	}
	const rowFormat = "cache-scale policy_block_bytes=%d policy_max_spans=%d " +
		"byte_limit=%d handle_limit=%d segments=%d entries_per_segment=%d " +
		"retained_block_bytes=%d retained_metadata_bytes=%d retained_bytes=%d " +
		"peak_retained_bytes=%d go_heap_bytes=%d go_heap_delta_bytes=%d " +
		"js_heap_bytes=%d js_heap_delta_bytes=%d js_heap_status=%s " +
		"live_handles=%d peak_handles=%d reads=%d fetched_bytes=%d hits=%d " +
		"misses=%d admissions=%d evictions=%d bypasses=%d release_errors=%d " +
		"wall_ms=%d"
	t.Logf(
		rowFormat,
		cachedSegmentBlockSize,
		maxCachedSegmentSpans,
		defaultCacheByteLimit,
		defaultCacheHandleLimit,
		count,
		entryCount,
		stats.BlockBytes,
		stats.MetadataBytes,
		stats.ChargedBytes,
		stats.PeakChargedBytes,
		goAfter.HeapAlloc,
		int64(goAfter.HeapAlloc)-int64(goBefore.HeapAlloc),
		jsAfter,
		jsDelta,
		jsStatus,
		stats.LiveHandles,
		stats.PeakLiveHandles,
		stats.ReadCalls,
		stats.FetchedBytes,
		stats.Hits,
		stats.Misses,
		stats.Admissions,
		stats.Evictions,
		stats.Bypasses,
		stats.ReleaseErrors,
		time.Since(started).Milliseconds(),
	)
	if stats.ChargedBytes > defaultCacheByteLimit {
		t.Fatalf("charged bytes exceeded limit: got %d limit %d", stats.ChargedBytes, defaultCacheByteLimit)
	}
	if stats.LiveHandles > defaultCacheHandleLimit {
		t.Fatalf("live handles exceeded limit: got %d limit %d", stats.LiveHandles, defaultCacheHandleLimit)
	}
	if stats.ReleaseErrors != 0 {
		t.Fatalf("driver release errors: got %d", stats.ReleaseErrors)
	}

	// Prove phase closeout drains every admitted resource.
	cache.close()
	closed := cache.snapshot()
	if closed.ChargedBytes != 0 || closed.LiveHandles != 0 {
		t.Fatalf("cache close left charged=%d handles=%d", closed.ChargedBytes, closed.LiveHandles)
	}
}

func cacheScaleJSHeap() (uint64, string) {
	performance := js.Global().Get("performance")
	if performance.Type() != js.TypeObject {
		return 0, "performance-unavailable"
	}
	memory := performance.Get("memory")
	if memory.Type() != js.TypeObject {
		return 0, "performance-memory-unavailable"
	}
	used := memory.Get("usedJSHeapSize")
	if used.Type() != js.TypeNumber {
		return 0, "used-js-heap-size-unavailable"
	}
	return uint64(used.Float()), "available"
}
