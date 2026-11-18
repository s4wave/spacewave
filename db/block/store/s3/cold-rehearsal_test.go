//go:build !tinygo

package block_store_s3

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// TestColdPackRehearsal measures the full-scale synthetic export. It is opt-in
// because generating and sorting 17 million real block hashes is not a unit test.
func TestColdPackRehearsal(t *testing.T) {
	// Reuse the same full fixture for the Go benchmark in this test process.
	f := requireRehearsalFixture(t)
	root := t.TempDir()
	opts := ColdPackOptions{
		Prefix: "cold/", ManifestFile: filepath.Join(root, "manifest"),
		PromotedLog: filepath.Join(root, "promoted"), PromoteCap: 500000,
	}
	data, err := f.manifest.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.ManifestFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("shape keys=%d parts=34 packs=%d bytes=%d tail_bytes=%d manifest_bytes=%d build_elapsed=%s",
		rehearsalKeys, len(f.catalog.packs), f.logicalBytes, f.tailBytes, len(data), f.buildTime)

	// Separate the existence, body and restart counters by mount.
	reput := f.measureExistence(t)
	reput += f.measureReads(t, opts)
	reput += f.measureRestart(t, opts)
	t.Logf("total uncached_existence_misses=%d (not a count of writes)", reput)
}

// requireRehearsalFixture loads the immutable writer fixture only when requested.
func requireRehearsalFixture(tb testing.TB) *rehearsalFixture {
	// Keep routine Go tests lightweight.
	tb.Helper()
	if os.Getenv("SPACEWAVE_COLD_REHEARSAL") != "1" {
		tb.Skip("set SPACEWAVE_COLD_REHEARSAL=1 for the full synthetic export")
	}
	f, err := loadRehearsalFixture()
	if err != nil {
		tb.Fatal(err)
	}
	return f
}

// observer isolates the exact production lookup from asynchronous tail filling.
// This keeps the before/after cache states and benchmark inputs repeatable.
func (f *rehearsalFixture) observer() *ColdPackStore {
	// Publish the actual writer catalog without starting an I/O worker.
	s := &ColdPackStore{
		client: f.bucket, bucket: "fake", ranges: make(chan struct{}, 8),
		negative: newColdCache[[sha256.Size]byte, struct{}](65536),
	}
	s.promotions = newColdPromotions(s, "", 0)
	s.tails = newColdTailCache(s, 64<<20)
	s.catalog.Store(f.catalog)
	return s
}

// measureExistence reports the exact per-reference bloom and existence answers.
func (f *rehearsalFixture) measureExistence(t *testing.T) uint64 {
	// Start with an empty resident LRU and an unserviced background queue.
	t.Helper()
	s := f.observer()
	ctx := t.Context()
	var overlap, bloomed [4]int
	absentByWindow := make([][]*block.BlockRef, (len(f.catalog.packs)+rehearsalWindow-1)/rehearsalWindow)
	var absentYes int
	before := f.bucket.requests.Load()

	// Count one positive per ref with one or more positive covering blooms.
	for _, ref := range f.absent {
		// Classify the interval shape separately from the production bloom count.
		width, window := 0, -1
		f.catalog.candidates(packfile.BlockKey(ref.GetHash()), func(pack uint16) bool {
			// Parts are disjoint; all covering packs fit the same warm window.
			num := int(pack) / rehearsalWindow
			if window != -1 && num != window {
				t.Fatalf("absent sample spans two disjoint export windows")
			}
			width++
			window = num
			return true
		})
		if width >= len(overlap) {
			t.Fatalf("unexpected overlapping pack count %d", width)
		}
		overlap[width]++
		hits := s.stats.bloomHits.Load()
		yes, err := s.GetBlockExists(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		if s.stats.bloomHits.Load() > hits {
			bloomed[width]++
			absentByWindow[window] = append(absentByWindow[window], ref)
		}
		if yes {
			absentYes++
		}
	}
	absentStats := s.Stats()
	t.Logf("absent n=%d overlap_0_1_2_3=%v positive_0_1_2_3=%v existence_yes=%d requests=%d uncached_existence_misses=%d",
		len(f.absent), overlap, bloomed, absentYes, f.bucket.requests.Load()-before, absentStats.UncachedExistenceMisses)
	if absentYes != 0 || f.bucket.requests.Load() != before {
		t.Fatal("absent existence answered yes or made a bucket request")
	}

	// Score the same 1,000 present refs before warming, then with their tails
	// resident in windows sized to the default 64 MiB LRU.
	beforeYes := countRehearsalYes(t, s, f.present)
	beforeStats := s.Stats()
	if beforeYes != 0 || f.bucket.requests.Load() != before {
		t.Fatal("empty existence cache answered present or made a bucket request")
	}
	afterYes, absentWarmYes, windows, warmRequests := f.measureResidentWindows(t, s, absentByWindow)
	t.Logf("present n=%d before_yes=%d after_yes=%d windows=%d warming_requests=%d cold_uncached_existence_misses=%d warm_uncached_existence_misses=%d",
		len(f.present), beforeYes, afterYes, windows, warmRequests,
		beforeStats.UncachedExistenceMisses-absentStats.UncachedExistenceMisses, s.Stats().UncachedExistenceMisses-beforeStats.UncachedExistenceMisses)
	t.Logf("absent_warm bloom_positive_checked=%d existence_yes=%d exact_tail_rejects=%d",
		bloomed[1]+bloomed[2]+bloomed[3], absentWarmYes, s.Stats().FalsePositives)
	if afterYes != len(f.present) || absentWarmYes != 0 {
		t.Fatal("resident exact index made a wrong existence decision")
	}
	return s.Stats().UncachedExistenceMisses
}

// rehearsalWindow holds two complete parts in the default 64 MiB LRU.
const rehearsalWindow = 2 * rehearsalPackCount

// measureResidentWindows verifies both present and bloom-positive absent refs
// while all their covering tails are resident.
func (f *rehearsalFixture) measureResidentWindows(t *testing.T, s *ColdPackStore, absent [][]*block.BlockRef) (yes, absentYes, windows int, requests uint64) {
	// Group present samples by their pack's position in the sorted manifest.
	t.Helper()
	bins := make([][]rehearsalSample, len(absent))
	for _, sample := range f.present {
		idx := f.ordinals[sample.object] / rehearsalWindow
		bins[idx] = append(bins[idx], sample)
	}

	// Warm two complete disjoint parts at a time under the default byte cap.
	for index, bin := range bins {
		// Load every covering pack before scoring either sample.
		resetRehearsalTails(s)
		before := f.bucket.requests.Load()
		start, end := index*rehearsalWindow, min((index+1)*rehearsalWindow, len(f.catalog.packs))
		for i := start; i < end; i++ {
			if _, err := s.tails.read(t.Context(), f.catalog, uint16(i)); err != nil {
				t.Fatal(err)
			}
		}
		for i := start; i < end; i++ {
			if s.tails.get(uint16(i)) == nil {
				t.Fatal("warm window exceeded the default tail budget")
			}
		}

		// Scoring presence and absence must cause no further range requests.
		warmed := f.bucket.requests.Load()
		yes += countRehearsalYes(t, s, bin)
		out, err := s.GetBlockExistsBatch(t.Context(), absent[index])
		if err != nil || f.bucket.requests.Load() != warmed {
			t.Fatalf("warm existence made a bucket request or returned an error: %v", err)
		}
		for _, found := range out {
			if found {
				absentYes++
			}
		}
		requests += warmed - before
		windows++
	}
	return yes, absentYes, windows, requests
}

// warm fetches the sampled pack indexes through the production tail path.
func (f *rehearsalFixture) warm(tb testing.TB, s *ColdPackStore, sample []rehearsalSample) {
	// Every measured ref must get a resident index, not merely a bloom hit.
	tb.Helper()
	catalog := s.catalog.Load()
	for _, item := range sample {
		if _, err := s.tails.read(tb.Context(), catalog, f.ordinals[item.object]); err != nil {
			tb.Fatal(err)
		}
	}
	for _, item := range sample {
		if s.tails.get(f.ordinals[item.object]) == nil {
			tb.Fatal("rehearsal warm window exceeded the tail budget")
		}
	}
}

// countRehearsalYes counts a batch's positive decisions, preserving input order.
func countRehearsalYes(t *testing.T, s *ColdPackStore, sample []rehearsalSample) int {
	// Score the real batch API while leaving all bucket work to the chosen setup.
	t.Helper()
	refs := make([]*block.BlockRef, len(sample))
	for i, item := range sample {
		refs[i] = item.ref
	}
	out, err := s.GetBlockExistsBatch(t.Context(), refs)
	if err != nil || len(out) != len(refs) {
		t.Fatalf("existence batch returned %d answers, error %v", len(out), err)
	}
	var count int
	for _, yes := range out {
		if yes {
			count++
		}
	}
	return count
}

// resetRehearsalTails makes a deliberate cold-cache control before a sample.
func resetRehearsalTails(s *ColdPackStore) {
	// Clear only the test mount's LRU and do not touch its manifest or proofs.
	cache := s.tails.resident
	cache.mtx.Lock()
	defer cache.mtx.Unlock()
	clear(cache.entries)
	cache.order.Init()
	cache.used = 0
}

// measureReads measures actual range counts and bytes then writes durable promotions.
func (f *rehearsalFixture) measureReads(t *testing.T, opts ColdPackOptions) uint64 {
	// Disable promotion only during the paired cold and resident-tail controls.
	t.Helper()
	without := opts
	without.PromotedLog = ""
	s, _ := f.open(t, without)
	cold, warm := make([]rehearsalIO, 0, len(f.present)), make([]rehearsalIO, 0, len(f.present))
	for _, item := range f.present {
		// Compare the same block with its tail absent and resident.
		resetRehearsalTails(s)
		cold = append(cold, f.sampleRead(t, s, item))
		warm = append(warm, f.sampleRead(t, s, item))
	}
	logRehearsalIO(t, "body_uncached", cold)
	logRehearsalIO(t, "body_cached_tail", warm)
	s.Close()

	// Promote all sampled keys with one bounded batch in flight at a time.
	p, _ := f.open(t, opts)
	count := 0
	for start := 0; start < len(f.present); start += 64 {
		// Wait outside the read path so queue drops cannot change the sample.
		for _, item := range f.present[start:min(start+64, len(f.present))] {
			f.sampleRead(t, p, item)
			count++
		}
		waitRehearsal(t, func() bool { return p.Stats().PromotedKeys == uint64(count) })
	}
	t.Logf("promote durable=%d drops=%d cap_drops=%d", p.Stats().PromotedKeys, p.Stats().PromoteDropped, p.Stats().PromoteCapDropped)
	p.Close()
	return s.Stats().UncachedExistenceMisses + p.Stats().UncachedExistenceMisses
}

// measureRestart separates CRC-log recovery from manifest preload and tests deletion.
func (f *rehearsalFixture) measureRestart(t *testing.T, opts ColdPackOptions) uint64 {
	// Time log open/recovery alone five times with the manifest already parsed.
	t.Helper()
	var timings []time.Duration
	for range 5 {
		p := newColdPromotions(&ColdPackStore{}, opts.PromotedLog, opts.PromoteCap)
		start := time.Now()
		log, err := p.open(opts.ManifestFile, f.catalog)
		timings = append(timings, time.Since(start))
		if err != nil || log == nil || p.store.Stats().PromotedKeys != rehearsalPresent {
			t.Fatalf("recover log: %v, promoted=%d", err, p.store.Stats().PromotedKeys)
		}
		if err := log.Close(); err != nil {
			t.Fatal(err)
		}
	}
	slices.Sort(timings)

	// Bound the restart cache below one tail to isolate restored extent hints.
	opts.TailCacheBytes = 1
	s, ready := f.open(t, opts)
	before := countRehearsalYes(t, s, f.present)
	var ranged []rehearsalIO
	for _, item := range f.present {
		ranged = append(ranged, f.sampleRead(t, s, item))
	}
	after := countRehearsalYes(t, s, f.present)
	if before != 0 || after != len(f.present) || s.Stats().TailFetches != 0 {
		t.Fatalf("unsafe restart membership: before=%d, after=%d, stats=%+v", before, after, s.Stats())
	}
	t.Logf("restart ready_elapsed=%s log_only_times=%v median=%s records=%d before_yes=%d verified_yes=%d tail_fetches=%d",
		ready, timings, timings[len(timings)/2], s.Stats().PromotedKeys, before, after, s.Stats().TailFetches)
	logRehearsalIO(t, "body_recovered_extent", ranged)

	// Observe an authoritative 404 after a known cached yes; positive evidence
	// for that object must stop authorizing puts to skip writing the block.
	victim := f.present[0]
	beforeStats := s.Stats()
	beforeBucket := f.bucket.requests.Load()
	beforeBytes := f.bucket.bytes.Load()
	existsBefore, err := s.GetBlockExists(t.Context(), victim.ref)
	if err != nil {
		t.Fatal(err)
	}
	f.bucket.removed.Store(&victim.object)
	defer f.bucket.removed.Store(nil)
	stored, readErr := s.GetStoredBlock(t.Context(), victim.ref)
	existsAfter, err := s.GetBlockExists(t.Context(), victim.ref)
	afterStats := s.Stats()
	if err != nil || readErr != nil || !existsBefore || stored != nil || existsAfter || len(afterStats.GonePackIDs) != 1 {
		t.Fatalf("404 fence: before=%v body=%v readErr=%v after=%v err=%v stats=%+v", existsBefore, stored != nil, readErr, existsAfter, err, afterStats)
	}
	t.Logf("removal exists_before=%v body_found=%v exists_after=%v requests=%d bytes=%d loaded=%d gone_ids=%v true_hits_delta=%d tail_fetches_delta=%d reput_delta=%d",
		existsBefore, stored != nil, existsAfter, f.bucket.requests.Load()-beforeBucket, f.bucket.bytes.Load()-beforeBytes,
		afterStats.LoadedPacks, afterStats.GonePackIDs, afterStats.TrueHits-beforeStats.TrueHits,
		afterStats.TailFetches-beforeStats.TailFetches, afterStats.UncachedExistenceMisses-beforeStats.UncachedExistenceMisses)
	s.Close()
	return afterStats.UncachedExistenceMisses
}

// open waits only in the harness for the real store to open its local files.
func (f *rehearsalFixture) open(t *testing.T, opts ColdPackOptions) (*ColdPackStore, time.Duration) {
	// Retain the mount until its measurement or test ends.
	t.Helper()
	start := time.Now()
	s, err := NewColdPackStore(t.Context(), nil, f.bucket, "fake", opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	waitRehearsal(t, func() bool { return s.Stats().Ready })
	return s, time.Since(start)
}

// waitRehearsal bounds measurement outside the request path.
func waitRehearsal(t *testing.T, ready func() bool) {
	// The API exposes a readback counter, not a test lifecycle notification.
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !ready() {
		select {
		case <-ctx.Done():
			t.Fatal("rehearsal background step did not publish")
		case <-tick.C:
		}
	}
}

// rehearsalIO records the fake bucket response costs of one body read.
type rehearsalIO struct {
	// requests is the number of range requests the read caused.
	requests uint64
	// bytes is the total bucket response body length.
	bytes uint64
}

// sampleRead takes the counter difference around one verified GetStoredBlock.
func (f *rehearsalFixture) sampleRead(t *testing.T, s *ColdPackStore, item rehearsalSample) rehearsalIO {
	// Require actual body bytes, not just a cached existence decision.
	t.Helper()
	requests, bytes := f.bucket.requests.Load(), f.bucket.bytes.Load()
	stored, err := s.GetStoredBlock(t.Context(), item.ref)
	body := stored.GetData()
	if err != nil || len(body) != rehearsalData || binary.BigEndian.Uint32(body[len(body)-4:]) != item.seed {
		t.Fatalf("body %s did not read its expected payload, length=%d, error=%v", item.ref.MarshalString(), len(body), err)
	}
	return rehearsalIO{requests: f.bucket.requests.Load() - requests, bytes: f.bucket.bytes.Load() - bytes}
}

// logRehearsalIO reports mean, p95, maximum and total range costs.
func logRehearsalIO(t *testing.T, label string, rows []rehearsalIO) {
	// Sort separately so even a duplicate-sized range counts once per read.
	t.Helper()
	requests, bytes := make([]uint64, len(rows)), make([]uint64, len(rows))
	var sumRequests, sumBytes uint64
	for i, row := range rows {
		requests[i], bytes[i] = row.requests, row.bytes
		sumRequests += row.requests
		sumBytes += row.bytes
	}
	slices.Sort(requests)
	slices.Sort(bytes)
	n, p95 := len(rows), (len(rows)*95+99)/100-1
	t.Logf("%s n=%d total_requests=%d avg_requests=%.4f p95_requests=%d max_requests=%d total_bytes=%d avg_bytes=%.1f p95_bytes=%d max_bytes=%d",
		label, n, sumRequests, float64(sumRequests)/float64(n), requests[p95], requests[n-1],
		sumBytes, float64(sumBytes)/float64(n), bytes[p95], bytes[n-1])
}

// BenchmarkColdPackRehearsalExists uses the same present refs with and without
// resident tails. It excludes startup, range I/O and background scheduling.
func BenchmarkColdPackRehearsalExists(b *testing.B) {
	// Choose a working set of sampled packs that fits the default LRU.
	f := requireRehearsalFixture(b)
	var sample []rehearsalSample
	for _, item := range f.present {
		if f.ordinals[item.object] < 36 {
			sample = append(sample, item)
		}
	}
	for _, name := range []string{"cold", "warm"} {
		b.Run(name, func(b *testing.B) {
			// Warm the same heads once before timing the exact per-reference path.
			s := f.observer()
			warm := name == "warm"
			if warm {
				f.warm(b, s, sample)
			}
			before := f.bucket.requests.Load()
			ctx := b.Context()
			b.ReportAllocs()
			i := 0

			// Time lookup only; neither measured state may issue a foreground range.
			for b.Loop() {
				yes, err := s.GetBlockExists(ctx, sample[i].ref)
				if yes != warm || err != nil {
					b.Fatalf("%s probe: %v, %v", name, yes, err)
				}
				i = (i + 1) % len(sample)
			}
			if f.bucket.requests.Load() != before {
				b.Fatal("benchmark issued a range")
			}
			b.ReportMetric(float64(len(sample)), "sample-refs")
		})
	}
}
