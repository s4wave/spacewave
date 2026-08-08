package block_gc

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"testing"

	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/cayley/graph/refs"
	"github.com/aperturerobotics/cayley/quad"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// BenchmarkResolveIRIRefKeysByExclusionSize measures operation-local IRI
// resolution across geometric exclusion sizes. Each leaf builds and seeds an
// isolated graph before timing the private post-442 resolver owner.
func BenchmarkResolveIRIRefKeysByExclusionSize(b *testing.B) {
	ctx := context.Background()
	for _, workload := range []string{"hit-only", "miss-only", "mixed", "duplicates"} {
		for _, path := range []struct {
			name     string
			fallback bool
		}{
			{name: "native-batch"},
			{name: "fallback-value", fallback: true},
		} {
			for _, exclusionSize := range iriWorkloadExclusionSizes() {
				name := workload + "/" + path.name + "/K=" + strconv.Itoa(exclusionSize)
				b.Run(name, func(b *testing.B) {
					b.ReportAllocs()
					b.StopTimer()
					workload := newIRIWorkloadBenchmark(b, ctx, workload, exclusionSize, path.fallback)
					if err := validateIRIWorkloadCallsite(ctx, workload, path.fallback); err != nil {
						b.Fatal(err)
					}

					heap := measureIRIWorkloadHeap(ctx, workload, path.fallback)
					if heap.err != nil {
						b.Fatal(heap.err)
					}
					b.ResetTimer()

					var total iriLookupStats
					for range b.N {
						workload.store.reset()
						b.StartTimer()
						keys, err := workload.graph.resolveIRIRefKeys(ctx, workload.inputs)
						b.StopTimer()
						if err := validateIRIWorkloadResult(workload, keys, err, path.fallback); err != nil {
							b.Fatal(err)
						}
						total.add(*workload.store.stats)
					}

					b.ReportMetric(float64(workload.exclusionSize), "input_K")
					b.ReportMetric(float64(workload.uniqueInputs), "unique_inputs")
					b.ReportMetric(float64(total.hits)/float64(b.N), "hits")
					b.ReportMetric(float64(total.misses)/float64(b.N), "misses")
					b.ReportMetric(float64(total.batchCalls)/float64(b.N), "batch_calls")
					b.ReportMetric(float64(total.fallbackCalls)/float64(b.N), "fallback_calls")
					b.ReportMetric(float64(total.returnedKeys)/float64(b.N), "returned_keys")
					b.ReportMetric(float64(total.lookupInputs)/float64(b.N), "lookup_inputs")
					b.ReportMetric(float64(workload.exclusionSize-workload.uniqueInputs), "duplicate_inputs")
					heap.report(b)
					b.Logf("source=private resolveIRIRefKeys; post-442 resident_iri_cache=absent; native_kv_zero_sentinel=preserved_and_measured; callsite=validated mandatory=%s; heap_sys=per-leaf baseline delta only, not cross-leaf comparable without subprocess isolation", NodeUnreferenced)
				})
			}
		}
	}
}

func iriWorkloadExclusionSizes() []int {
	return []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096}
}

type iriWorkloadBenchmark struct {
	graph            *RefGraph
	store            *iriCountingStore
	inputs           []string
	exclusionSize    int
	uniqueInputs     int
	expectedHits     int
	expectedMisses   int
	expectedKeys     map[any]struct{}
	expectedReturned int
}

func newIRIWorkloadBenchmark(
	b *testing.B,
	ctx context.Context,
	workload string,
	exclusionSize int,
	fallback bool,
) *iriWorkloadBenchmark {
	b.Helper()
	graph, err := NewRefGraph(ctx, store_kvtx_inmem.NewStore(), []byte("gc/"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := graph.Close(); err != nil {
			b.Error(err)
		}
	})

	inputs, hitNames, expectedHits := iriWorkloadInputs(workload, exclusionSize)
	seed := make([]RefEdge, 0, len(hitNames)+2)
	seed = append(seed,
		RefEdge{Subject: "iri-workload/callsite-owner", Object: "iri-workload/callsite-target"},
		RefEdge{Subject: NodeUnreferenced, Object: "iri-workload/callsite-unreferenced"},
	)
	for i, iri := range hitNames {
		seed = append(seed, RefEdge{
			Subject: iri,
			Object:  "iri-workload/hit-target/" + strconv.Itoa(i),
		})
	}
	if err := graph.ApplyRefBatch(ctx, seed, nil); err != nil {
		b.Fatal(err)
	}
	underlyingStore := graph.handle.QuadStore

	expectedKeys := make(map[any]struct{}, len(hitNames))
	uniqueHitNames := make(map[string]struct{}, len(hitNames))
	for _, iri := range hitNames {
		if _, ok := uniqueHitNames[iri]; ok {
			continue
		}
		uniqueHitNames[iri] = struct{}{}
		ref, err := underlyingStore.ValueOf(ctx, quad.IRI(iri))
		if err != nil {
			b.Fatalf("resolve expected IRI ref key %q: %v", iri, err)
		}
		if ref == nil {
			b.Fatalf("seeded hit IRI %q resolved to nil", iri)
		}
		expectedKeys[refs.ToKey(ref)] = struct{}{}
	}
	if !fallback && exclusionSize > expectedHits {
		expectedKeys[cayley_kv.Int64Value(0)] = struct{}{}
	}

	stats := new(iriLookupStats)
	base := &iriCountingStore{QuadStore: underlyingStore, stats: stats, expectedLookup: len(inputs)}
	var store *iriCountingStore
	if fallback {
		graph.handle.QuadStore = &iriFallbackQuadStore{iriCountingStore: base}
		store = base
	} else {
		graph.handle.QuadStore = &iriBatchQuadStore{iriCountingStore: base}
		store = base
	}
	unique := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		unique[input] = struct{}{}
	}
	b.Logf("fixture=isolated seed_edges=%d", len(seed))
	return &iriWorkloadBenchmark{
		graph:            graph,
		store:            store,
		inputs:           inputs,
		exclusionSize:    exclusionSize,
		uniqueInputs:     len(unique),
		expectedHits:     expectedHits,
		expectedMisses:   exclusionSize - expectedHits,
		expectedKeys:     expectedKeys,
		expectedReturned: len(expectedKeys),
	}
}

func iriWorkloadInputs(workload string, exclusionSize int) ([]string, []string, int) {
	hitName := func(i int) string {
		return "iri-workload/hit/" + strconv.Itoa(i)
	}
	missName := func(i int) string {
		return "iri-workload/miss/" + strconv.Itoa(i)
	}

	switch workload {
	case "hit-only":
		inputs := make([]string, exclusionSize)
		hits := make([]string, exclusionSize)
		for i := range inputs {
			inputs[i] = hitName(i)
			hits[i] = inputs[i]
		}
		return inputs, hits, exclusionSize
	case "miss-only":
		inputs := make([]string, exclusionSize)
		for i := range inputs {
			inputs[i] = missName(i)
		}
		return inputs, nil, 0
	case "mixed":
		hitCount := (exclusionSize + 1) / 2
		inputs := make([]string, exclusionSize)
		hits := make([]string, hitCount)
		for i := range inputs {
			if i < hitCount {
				inputs[i] = hitName(i)
				hits[i] = inputs[i]
				continue
			}
			inputs[i] = missName(i - hitCount)
		}
		return inputs, hits, hitCount
	case "duplicates":
		uniqueCount := (exclusionSize + 1) / 2
		inputs := make([]string, exclusionSize)
		hits := make([]string, uniqueCount)
		for i := range hits {
			hits[i] = hitName(i)
		}
		for i := range inputs {
			inputs[i] = hits[i%uniqueCount]
		}
		return inputs, hits, exclusionSize
	default:
		panic("unknown IRI workload: " + workload)
	}
}

func validateIRIWorkloadCallsite(ctx context.Context, workload *iriWorkloadBenchmark, fallback bool) error {
	workload.store.reset()
	found, err := workload.graph.HasIncomingRefsExcluding(
		ctx,
		"iri-workload/callsite-target",
		"iri-workload/callsite-owner",
	)
	if err != nil {
		return err
	}
	if found {
		return errors.New("fixed candidate graph was not excluded")
	}
	if workload.store.stats.lookupInputs < 2 || workload.store.stats.hits < 2 || workload.store.stats.misses != 0 {
		return errors.New("callsite naming accounting did not include the mandatory unreferenced exclusion")
	}
	if fallback && workload.store.stats.fallbackCalls < 2 {
		return errors.New(
			"callsite did not exercise the per-value fallback: fallback=" +
				strconv.Itoa(workload.store.stats.fallbackCalls) +
				" inputs=" + strconv.Itoa(workload.store.stats.lookupInputs),
		)
	}
	if !fallback && workload.store.stats.batchCalls != 1 {
		return errors.New("callsite did not exercise the native batch path")
	}
	return nil
}

func TestValidateIRIWorkloadKeySetRejectsWrongKey(t *testing.T) {
	expected := map[any]struct{}{"expected": {}}
	got := map[any]struct{}{"wrong": {}}
	if err := validateIRIWorkloadKeySet(expected, got); err == nil {
		t.Fatal("wrong returned key passed identity validation")
	}
}

func validateIRIWorkloadKeySet(expected, got map[any]struct{}) error {
	if len(got) != len(expected) {
		return errors.New(
			"returned keys = " + strconv.Itoa(len(got)) +
				", want " + strconv.Itoa(len(expected)),
		)
	}
	for key := range expected {
		if _, ok := got[key]; !ok {
			return errors.New("returned keys omitted an expected key")
		}
	}
	for key := range got {
		if _, ok := expected[key]; !ok {
			return errors.New("returned keys contained an unexpected key")
		}
	}
	return nil
}

func validateIRIWorkloadResult(
	workload *iriWorkloadBenchmark,
	keys map[any]struct{},
	err error,
	fallback bool,
) error {
	if err != nil {
		return err
	}
	if got := residentIRIRefKeyCount(workload.graph); got != 0 {
		return errors.New("post-442 resolver retained " + strconv.Itoa(got) + " IRI ref keys")
	}
	if err := validateIRIWorkloadKeySet(workload.expectedKeys, keys); err != nil {
		return err
	}
	workload.store.stats.returnedKeys = len(keys)
	return workload.store.stats.assert(
		workload.expectedHits,
		workload.expectedMisses,
		workload.expectedReturned,
		fallback,
	)
}

func measureIRIWorkloadHeap(
	ctx context.Context,
	workload *iriWorkloadBenchmark,
	fallback bool,
) iriWorkloadHeapProbe {
	workload.store.reset()
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)

	var afterLookup runtime.MemStats
	workload.store.afterLookupProbe = func() {
		runtime.ReadMemStats(&afterLookup)
	}
	keys, err := workload.graph.resolveIRIRefKeys(ctx, workload.inputs)
	workload.store.afterLookupProbe = nil
	if err != nil {
		return iriWorkloadHeapProbe{err: err}
	}
	var returned runtime.MemStats
	runtime.ReadMemStats(&returned)
	runtime.GC()
	var postGC runtime.MemStats
	runtime.ReadMemStats(&postGC)
	if !workload.store.afterLookupCalled {
		return iriWorkloadHeapProbe{err: errors.New("heap probe did not observe the final name lookup")}
	}
	if err := validateIRIWorkloadResult(workload, keys, nil, fallback); err != nil {
		return iriWorkloadHeapProbe{err: err}
	}
	return iriWorkloadHeapProbe{
		baseline:    iriHeapSampleOf(baseline),
		afterLookup: iriHeapSampleOf(afterLookup),
		returned:    iriHeapSampleOf(returned),
		postGC:      iriHeapSampleOf(postGC),
	}
}

type iriLookupStats struct {
	lookupInputs  int
	hits          int
	misses        int
	batchCalls    int
	fallbackCalls int
	returnedKeys  int
}

func (s *iriLookupStats) reset() {
	*s = iriLookupStats{}
}

func (s *iriLookupStats) add(other iriLookupStats) {
	s.lookupInputs += other.lookupInputs
	s.hits += other.hits
	s.misses += other.misses
	s.batchCalls += other.batchCalls
	s.fallbackCalls += other.fallbackCalls
	s.returnedKeys += other.returnedKeys
}

func (s *iriLookupStats) assert(expectedHits, expectedMisses, expectedReturned int, fallback bool) error {
	if s.lookupInputs != expectedHits+expectedMisses {
		return errors.New("lookup inputs did not match hits plus misses")
	}
	if s.hits != expectedHits || s.misses != expectedMisses {
		return errors.New("hit and miss accounting did not match workload")
	}
	if s.batchCalls != boolInt(!fallback) || s.fallbackCalls != (expectedHits+expectedMisses)*boolInt(fallback) {
		return errors.New(
			"batch and fallback call accounting did not match path: batch=" +
				strconv.Itoa(s.batchCalls) +
				" fallback=" + strconv.Itoa(s.fallbackCalls),
		)
	}
	if s.returnedKeys != expectedReturned {
		return errors.New("returned key accounting did not match resolver result")
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type iriCountingStore struct {
	graph.QuadStore
	stats             *iriLookupStats
	expectedLookup    int
	afterLookupProbe  func()
	afterLookupCalled bool
}

func (s *iriCountingStore) reset() {
	s.stats.reset()
	s.afterLookupCalled = false
}

func (s *iriCountingStore) ValueOf(ctx context.Context, value quad.Value) (graph.Ref, error) {
	s.stats.fallbackCalls++
	s.stats.lookupInputs++
	ref, err := s.QuadStore.ValueOf(ctx, value)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		s.stats.misses++
	} else {
		s.stats.hits++
	}
	s.maybeAfterLookup()
	return ref, nil
}

func (s *iriCountingStore) countBatchResult(nodes []quad.Value, resolved []graph.Ref) error {
	if len(nodes) != len(resolved) {
		return errors.New("RefsOf returned a non-aligned result")
	}
	s.stats.lookupInputs += len(nodes)
	for _, ref := range resolved {
		if iriBatchRefIsMissing(ref) {
			s.stats.misses++
		} else {
			s.stats.hits++
		}
	}
	s.maybeAfterLookup()
	return nil
}

func iriBatchRefIsMissing(ref graph.Ref) bool {
	if ref == nil {
		return true
	}
	key, ok := refs.ToKey(ref).(cayley_kv.Int64Value)
	return ok && key == 0
}

func (s *iriCountingStore) maybeAfterLookup() {
	if s.afterLookupProbe == nil || s.afterLookupCalled || s.stats.lookupInputs != s.expectedLookup {
		return
	}
	s.afterLookupCalled = true
	s.afterLookupProbe()
}

type iriBatchQuadStore struct {
	*iriCountingStore
}

var (
	_ graph.QuadStore = (*iriBatchQuadStore)(nil)
	_ refs.BatchNamer = (*iriBatchQuadStore)(nil)
)

func (s *iriBatchQuadStore) RefsOf(ctx context.Context, nodes []quad.Value) ([]graph.Ref, error) {
	s.stats.batchCalls++
	resolved, err := s.QuadStore.(refs.BatchNamer).RefsOf(ctx, nodes)
	if err != nil {
		return nil, err
	}
	// Preserve Cayley KV's zero sentinel in the returned slice; the benchmark
	// counts it as a miss instead of normalizing production output.
	if err := s.countBatchResult(nodes, resolved); err != nil {
		return nil, err
	}
	return resolved, nil
}

func (s *iriBatchQuadStore) ValuesOf(ctx context.Context, values []graph.Ref) ([]quad.Value, error) {
	return s.QuadStore.(refs.BatchNamer).ValuesOf(ctx, values)
}

type iriFallbackQuadStore struct {
	*iriCountingStore
}

var _ graph.QuadStore = (*iriFallbackQuadStore)(nil)

// iriWorkloadHeapProbe samples one untimed resolver operation per leaf.
type iriWorkloadHeapProbe struct {
	err         error
	baseline    iriHeapSample
	afterLookup iriHeapSample
	returned    iriHeapSample
	postGC      iriHeapSample
}

type iriHeapSample struct {
	alloc uint64
	inuse uint64
	sys   uint64
}

func iriHeapSampleOf(stats runtime.MemStats) iriHeapSample {
	return iriHeapSample{alloc: stats.HeapAlloc, inuse: stats.HeapInuse, sys: stats.HeapSys}
}

func (s iriHeapSample) deltaFrom(baseline iriHeapSample) iriHeapDelta {
	return iriHeapDelta{
		alloc: iriSignedHeapDelta(s.alloc, baseline.alloc),
		inuse: iriSignedHeapDelta(s.inuse, baseline.inuse),
		sys:   iriSignedHeapDelta(s.sys, baseline.sys),
	}
}

func iriSignedHeapDelta(value, baseline uint64) float64 {
	if value >= baseline {
		return float64(value - baseline)
	}
	return -float64(baseline - value)
}

type iriHeapDelta struct {
	alloc float64
	inuse float64
	sys   float64
}

func (s *iriHeapSample) max(other iriHeapSample) {
	if other.alloc > s.alloc {
		s.alloc = other.alloc
	}
	if other.inuse > s.inuse {
		s.inuse = other.inuse
	}
	if other.sys > s.sys {
		s.sys = other.sys
	}
}

func (p iriWorkloadHeapProbe) report(b *testing.B) {
	b.ReportMetric(float64(p.baseline.alloc), "heap_seed_baseline_alloc_bytes")
	b.ReportMetric(float64(p.baseline.inuse), "heap_seed_baseline_inuse_bytes")
	b.ReportMetric(float64(p.baseline.sys), "heap_seed_baseline_sys_bytes")

	afterLookup := p.afterLookup.deltaFrom(p.baseline)
	b.ReportMetric(afterLookup.alloc, "heap_after_name_lookup_delta_alloc_bytes")
	b.ReportMetric(afterLookup.inuse, "heap_after_name_lookup_delta_inuse_bytes")
	b.ReportMetric(afterLookup.sys, "heap_after_name_lookup_per_leaf_delta_sys_bytes")

	returned := p.returned.deltaFrom(p.baseline)
	b.ReportMetric(returned.alloc, "heap_returned_delta_alloc_bytes")
	b.ReportMetric(returned.inuse, "heap_returned_delta_inuse_bytes")
	b.ReportMetric(returned.sys, "heap_returned_per_leaf_delta_sys_bytes")

	peak := p.baseline
	peak.max(p.afterLookup)
	peak.max(p.returned)
	peakDelta := peak.deltaFrom(p.baseline)
	b.ReportMetric(peakDelta.alloc, "heap_peak_delta_alloc_bytes")
	b.ReportMetric(peakDelta.inuse, "heap_peak_delta_inuse_bytes")
	b.ReportMetric(peakDelta.sys, "heap_peak_per_leaf_delta_sys_bytes")

	postGC := p.postGC.deltaFrom(p.baseline)
	b.ReportMetric(postGC.alloc, "heap_post_gc_delta_live_alloc_bytes")
	b.ReportMetric(postGC.inuse, "heap_post_gc_delta_inuse_bytes")
	b.ReportMetric(postGC.sys, "heap_post_gc_per_leaf_delta_sys_bytes")
}
