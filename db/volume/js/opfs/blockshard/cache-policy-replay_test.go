//go:build js

package blockshard

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"runtime"
	"slices"
	"syscall/js"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

const cachePolicyReplayEnv = "SPACEWAVE_OPFS_CACHE_POLICY_REPLAY"

// cachePolicyReplayFixture describes one immutable real-OPFS segment corpus.
type cachePolicyReplayFixture struct {
	filename string
	data     []byte
	meta     *SegmentMeta
	keys     [][]byte
	value    []byte
}

// cachePolicyReplayWorkload fixes one request order over an immutable fixture.
type cachePolicyReplayWorkload struct {
	name    string
	fixture *cachePolicyReplayFixture
	keys    [][]byte
}

// cachePolicyReplaySample retains one complete policy and workload observation.
type cachePolicyReplaySample struct {
	wall         time.Duration
	goHeapDelta  int64
	jsHeapDelta  int64
	jsHeapStatus string
	stats        cacheStats
}

func TestCachePolicyReplay(t *testing.T) {
	// Keep the real-OPFS replay out of routine package checks.
	if os.Getenv(cachePolicyReplayEnv) != "1" {
		t.Skipf("set %s=1 to run the cache policy replay", cachePolicyReplayEnv)
	}

	// Create the point-scan and near-threshold immutable segment fixtures.
	point := newCachePolicyReplayFixture(t, "point.sst", 4096, 176)
	window := newCachePolicyReplayFixture(t, "window.sst", 64, 12<<10)
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	const dirName = "blockshard-cache-policy-replay"
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
	for _, fixture := range []*cachePolicyReplayFixture{point, window} {
		if err := opfs.WriteFile(dir, fixture.filename, fixture.data); err != nil {
			t.Fatal(err)
		}
	}

	// Exercise fixed sequential, strided, hotset, and threshold-window orders.
	workloads := []cachePolicyReplayWorkload{
		{name: "point-sequential", fixture: point, keys: point.keys},
		{name: "point-strided", fixture: point, keys: cachePolicyReplayStridedKeys(point.keys, 2053)},
		{name: "point-hotset", fixture: point, keys: cachePolicyReplayRepeatedKeys(point.keys[2048:2112], 64)},
		{name: "window-hot", fixture: window, keys: cachePolicyReplayRepeatedKeys(window.keys[32:33], 64)},
	}
	for _, workload := range workloads {
		runCachePolicyReplayWorkload(t, dir, workload)
	}
}

// runCachePolicyReplayWorkload records ten fresh-cache samples and one summary.
func runCachePolicyReplayWorkload(
	t *testing.T,
	dir js.Value,
	workload cachePolicyReplayWorkload,
) {
	t.Helper()
	const repetitions = 10
	samples := make([]cachePolicyReplaySample, repetitions)

	// Run every repetition through a fresh coordinator and snapshot identity.
	for repetition := range repetitions {
		runtime.GC()
		var goBefore runtime.MemStats
		runtime.ReadMemStats(&goBefore)
		jsBefore, jsStatus := cacheScaleJSHeap()
		cache := newDefaultCacheCoordinator()
		started := time.Now()

		// Resolve every key through the production lease, lookup, and fill path.
		for _, key := range workload.keys {
			lease, err := cache.acquireSegment(
				context.Background(),
				cacheKey{shardID: 0, filename: workload.fixture.filename},
				workload.fixture.meta,
				func() (segmentReader, error) {
					return opfs.OpenReadSnapshot(dir, workload.fixture.filename)
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			value, found, err := lease.lookup.Get(lease, key)
			lease.Release()
			if err != nil {
				t.Fatal(err)
			}
			if !found || !bytes.Equal(value, workload.fixture.value) {
				t.Fatalf("workload %s returned found=%t bytes=%d", workload.name, found, len(value))
			}
		}
		wall := time.Since(started)

		// Capture retained cache and heap state before closing the coordinator.
		runtime.GC()
		var goAfter runtime.MemStats
		runtime.ReadMemStats(&goAfter)
		jsAfter, afterStatus := cacheScaleJSHeap()
		if afterStatus != jsStatus {
			jsStatus += "-then-" + afterStatus
		}
		jsDelta := int64(0)
		if jsStatus == "available" {
			jsDelta = int64(jsAfter) - int64(jsBefore)
		}
		sample := cachePolicyReplaySample{
			wall:         wall,
			goHeapDelta:  int64(goAfter.HeapAlloc) - int64(goBefore.HeapAlloc),
			jsHeapDelta:  jsDelta,
			jsHeapStatus: jsStatus,
			stats:        cache.snapshot(),
		}

		// Close every admitted resource and retain the drained counters.
		cache.close()
		closed := cache.snapshot()
		if closed.ChargedBytes != 0 || closed.LiveHandles != 0 {
			t.Fatalf(
				"workload %s close left charged=%d handles=%d",
				workload.name,
				closed.ChargedBytes,
				closed.LiveHandles,
			)
		}
		samples[repetition] = sample
		logCachePolicyReplaySample(t, workload.name, repetition, sample)
	}

	// Report nearest-rank latency summaries for policy qualification.
	walls := make([]time.Duration, len(samples))
	for i := range samples {
		walls[i] = samples[i].wall
	}
	slices.Sort(walls)
	median := walls[(len(walls)-1)/2]
	p95 := walls[(95*len(walls)+99)/100-1]
	t.Logf(
		"cache-policy-summary block_bytes=%d max_spans=%d threshold_bytes=%d workload=%s repetitions=%d median_us=%d p95_us=%d",
		cachedSegmentBlockSize,
		maxCachedSegmentSpans,
		maxCachedSegmentRead,
		workload.name,
		len(samples),
		median.Microseconds(),
		p95.Microseconds(),
	)
}

// logCachePolicyReplaySample emits one parseable source row for durable evidence.
func logCachePolicyReplaySample(
	t *testing.T,
	workload string,
	repetition int,
	sample cachePolicyReplaySample,
) {
	t.Helper()
	stats := sample.stats
	t.Logf(
		"cache-policy-sample block_bytes=%d max_spans=%d threshold_bytes=%d workload=%s repetition=%d wall_us=%d retained_block_bytes=%d retained_metadata_bytes=%d retained_bytes=%d peak_retained_bytes=%d go_heap_delta_bytes=%d js_heap_delta_bytes=%d js_heap_status=%s live_handles=%d peak_handles=%d reads=%d fetched_bytes=%d hits=%d misses=%d admissions=%d evictions=%d bypasses=%d shared_fills=%d release_errors=%d",
		cachedSegmentBlockSize,
		maxCachedSegmentSpans,
		maxCachedSegmentRead,
		workload,
		repetition,
		sample.wall.Microseconds(),
		stats.BlockBytes,
		stats.MetadataBytes,
		stats.ChargedBytes,
		stats.PeakChargedBytes,
		sample.goHeapDelta,
		sample.jsHeapDelta,
		sample.jsHeapStatus,
		stats.LiveHandles,
		stats.PeakLiveHandles,
		stats.ReadCalls,
		stats.FetchedBytes,
		stats.Hits,
		stats.Misses,
		stats.Admissions,
		stats.Evictions,
		stats.Bypasses,
		stats.SharedFills,
		stats.ReleaseErrors,
	)
}

// newCachePolicyReplayFixture builds one deterministic SSTable and its metadata.
func newCachePolicyReplayFixture(
	t testing.TB,
	filename string,
	entryCount int,
	valueSize int,
) *cachePolicyReplayFixture {
	t.Helper()
	entries := make([]segment.Entry, entryCount)
	keys := make([][]byte, entryCount)
	value := bytes.Repeat([]byte("v"), valueSize)
	for i := range entries {
		key := make([]byte, 4)
		binary.BigEndian.PutUint32(key, uint32(i))
		keys[i] = key
		entries[i] = segment.Entry{Key: key, Value: value}
	}
	data := buildSegmentData(t, entries)
	return &cachePolicyReplayFixture{
		filename: filename,
		data:     data,
		meta: &SegmentMeta{
			Filename:   filename,
			EntryCount: uint32(entryCount),
			Size:       uint32(len(data)),
			MinKey:     keys[0],
			MaxKey:     keys[len(keys)-1],
		},
		keys:  keys,
		value: value,
	}
}

// cachePolicyReplayStridedKeys returns one deterministic full-corpus permutation.
func cachePolicyReplayStridedKeys(keys [][]byte, stride int) [][]byte {
	out := make([][]byte, len(keys))
	for i := range out {
		out[i] = keys[(i*stride)%len(keys)]
	}
	return out
}

// cachePolicyReplayRepeatedKeys repeats one fixed hot set in source order.
func cachePolicyReplayRepeatedKeys(keys [][]byte, repetitions int) [][]byte {
	out := make([][]byte, 0, len(keys)*repetitions)
	for range repetitions {
		out = append(out, keys...)
	}
	return out
}
