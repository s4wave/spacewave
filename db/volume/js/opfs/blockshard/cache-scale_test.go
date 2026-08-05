//go:build js

package blockshard

import (
	"encoding/binary"
	"os"
	"runtime"
	"strconv"
	"syscall/js"
	"testing"
	"time"
	"unsafe"

	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

const cacheScaleSegmentsEnv = "SPACEWAVE_OPFS_CACHE_SCALE_SEGMENTS"

type cacheScaleReader struct {
	segmentReader
	reads int64
	bytes int64
}

func (r *cacheScaleReader) ReadAt(p []byte, off int64) (int, error) {
	r.reads++
	n, err := r.segmentReader.ReadAt(p, off)
	r.bytes += int64(n)
	return n, err
}

type cacheScaleSegment struct {
	file   *opfs.AsyncFile
	reader *cacheScaleReader
	cache  *cachedSegmentFile
	lookup *segment.LookupMeta
}

func TestCachedSegmentFileScale(t *testing.T) {
	// Select one isolated geometric corpus size.
	countText := os.Getenv(cacheScaleSegmentsEnv)
	if countText == "" {
		t.Skipf("set %s to the active segment count", cacheScaleSegmentsEnv)
	}
	count, err := strconv.Atoi(countText)
	if err != nil || count < 1 {
		t.Fatalf("%s=%q is not a positive integer", cacheScaleSegmentsEnv, countText)
	}

	// Build one deterministic SSTable shared by every active cache entry.
	entries := make([]segment.Entry, 4096)
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

	// Capture the runtime baseline before retaining cache resources.
	runtime.GC()
	var goBefore runtime.MemStats
	runtime.ReadMemStats(&goBefore)
	jsBefore, jsStatus := cacheScaleJSHeap()
	started := time.Now()
	segments := make([]cacheScaleSegment, 0, count)
	t.Cleanup(func() {
		for i := range segments {
			if err := segments[i].file.Close(); err != nil {
				t.Error(err)
			}
		}
	})

	// Open and touch one independent cache entry per active segment.
	for i := range count {
		file, err := opfs.OpenAsyncFile(dir, filename)
		if err != nil {
			t.Fatalf("open segment %d: %v", i, err)
		}
		reader := &cacheScaleReader{segmentReader: file}
		cache := newCachedSegmentFile(reader, int64(len(data)))
		lookup, err := segment.LoadLookupMeta(cache, int64(len(data)))
		if err != nil {
			file.Close()
			t.Fatalf("load segment %d metadata: %v", i, err)
		}
		key := entries[(i*2053)%len(entries)].Key
		got, found, err := lookup.Get(cache, key)
		if err != nil {
			file.Close()
			t.Fatalf("read segment %d: %v", i, err)
		}
		if !found || len(got) != len(value) {
			file.Close()
			t.Fatalf("read segment %d returned found=%t bytes=%d", i, found, len(got))
		}
		segments = append(segments, cacheScaleSegment{
			file:   file,
			reader: reader,
			cache:  cache,
			lookup: lookup,
		})
	}

	// Capture heaps while every segment resource remains reachable.
	runtime.GC()
	var goAfter runtime.MemStats
	runtime.ReadMemStats(&goAfter)
	jsAfter, afterStatus := cacheScaleJSHeap()
	if afterStatus != jsStatus {
		jsStatus += "-then-" + afterStatus
	}

	// Count exact retained payloads and underlying I/O.
	var blockBytes uint64
	var metadataBytes uint64
	var reads int64
	var fetchedBytes int64
	for i := range segments {
		segments[i].cache.mu.Lock()
		for _, block := range segments[i].cache.blocks {
			blockBytes += uint64(cap(block))
		}
		segments[i].cache.mu.Unlock()
		metadataBytes += cacheScaleLookupBytes(segments[i].lookup)
		reads += segments[i].reader.reads
		fetchedBytes += segments[i].reader.bytes
	}

	// Report one parseable row for the geometric evidence table.
	jsDelta := int64(0)
	if jsStatus == "available" {
		jsDelta = int64(jsAfter) - int64(jsBefore)
	}
	t.Logf("cache-scale policy_block_bytes=%d policy_max_blocks=%d segments=%d retained_block_bytes=%d retained_metadata_bytes=%d go_heap_bytes=%d go_heap_delta_bytes=%d js_heap_bytes=%d js_heap_delta_bytes=%d js_heap_status=%s live_handles=%d reads=%d fetched_bytes=%d wall_ms=%d", cachedSegmentBlockSize, maxCachedSegmentBlocks, count, blockBytes, metadataBytes, goAfter.HeapAlloc, int64(goAfter.HeapAlloc)-int64(goBefore.HeapAlloc), jsAfter, jsDelta, jsStatus, len(segments), reads, fetchedBytes, time.Since(started).Milliseconds())
}

func cacheScaleLookupBytes(meta *segment.LookupMeta) uint64 {
	bytes := uint64(unsafe.Sizeof(*meta))
	bytes += uint64(unsafe.Sizeof(*meta.Header))
	bytes += uint64(cap(meta.MinKey) + cap(meta.MaxKey))
	bytes += uint64(cap(meta.Index)) * uint64(unsafe.Sizeof(segment.IndexEntry{}))
	for i := range meta.Index {
		bytes += uint64(cap(meta.Index[i].Key))
	}
	if meta.Bloom != nil {
		bytes += uint64(unsafe.Sizeof(*meta.Bloom))
		if meta.Header.BloomSize >= 5 {
			bytes += uint64(meta.Header.BloomSize - 5)
		}
	}
	return bytes
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
