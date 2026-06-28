package file

import (
	"bytes"
	"context"
	"io"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	bucket_mock "github.com/s4wave/spacewave/db/bucket/mock"
)

func metricBlobOpts() *blob.BuildBlobOpts {
	return &blob.BuildBlobOpts{
		RawHighWaterMark: 1,
		ChunkerArgs: &blob.ChunkerArgs{
			ChunkerType: blob.ChunkerType_ChunkerType_JC,
			JcArgs: &blob.JcArgs{
				ChunkingMinSize:    64,
				ChunkingTargetSize: 96,
				ChunkingMaxSize:    128,
			},
		},
	}
}

type metricRecorder struct {
	metrics []blob.Metric
}

func (r *metricRecorder) RecordBlobMetric(metric blob.Metric) {
	r.metrics = append(r.metrics, metric)
}

func (r *metricRecorder) reset() {
	r.metrics = nil
}

func (r *metricRecorder) stage(stage string) (int, int) {
	var count, bytes int
	for _, metric := range r.metrics {
		if metric.Stage != stage {
			continue
		}
		count++
		bytes += metric.ChunkBytes
	}
	return count, bytes
}

type metricCountingStore struct {
	block.StoreOps

	mtx         sync.Mutex
	putCalls    int
	putBatches  int
	putExisted  int
	putBytes    int
	getCalls    int
	getBytes    int
	existsCalls int
	existsHits  int
	readByRef   map[string]int
}

type metricStoreSnapshot struct {
	putCalls    int
	putBatches  int
	putExisted  int
	putBytes    int
	getCalls    int
	getBytes    int
	existsCalls int
	existsHits  int
	readByRef   map[string]int
}

func newMetricCountingStore() *metricCountingStore {
	return &metricCountingStore{
		StoreOps:  block_mock.NewMockStore(0),
		readByRef: make(map[string]int),
	}
}

func (s *metricCountingStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, existed, err := s.StoreOps.PutBlock(ctx, data, opts)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if err == nil {
		s.putCalls++
		s.putBytes += len(data)
		if existed {
			s.putExisted++
		}
	}
	return ref, existed, err
}

func (s *metricCountingStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	err := s.StoreOps.PutBlockBatch(ctx, entries)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if err == nil {
		s.putBatches++
		s.putCalls += len(entries)
		for _, entry := range entries {
			if entry != nil {
				s.putBytes += len(entry.Data)
			}
		}
	}
	return err
}

func (s *metricCountingStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := s.StoreOps.GetBlock(ctx, ref)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if err == nil {
		s.getCalls++
		if found {
			s.getBytes += len(data)
			if s.readByRef == nil {
				s.readByRef = make(map[string]int)
			}
			s.readByRef[ref.MarshalString()]++
		}
	}
	return data, found, err
}

func (s *metricCountingStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	exists, err := s.StoreOps.GetBlockExists(ctx, ref)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if err == nil {
		s.existsCalls++
		if exists {
			s.existsHits++
		}
	}
	return exists, err
}

func (s *metricCountingStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	exists, err := s.StoreOps.GetBlockExistsBatch(ctx, refs)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if err == nil {
		s.existsCalls += len(refs)
		for _, ok := range exists {
			if ok {
				s.existsHits++
			}
		}
	}
	return exists, err
}

func (s *metricCountingStore) reset() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.putCalls = 0
	s.putBatches = 0
	s.putExisted = 0
	s.putBytes = 0
	s.getCalls = 0
	s.getBytes = 0
	s.existsCalls = 0
	s.existsHits = 0
	s.readByRef = make(map[string]int)
}

func (s *metricCountingStore) snapshot() metricStoreSnapshot {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	readByRef := make(map[string]int, len(s.readByRef))
	maps.Copy(readByRef, s.readByRef)
	return metricStoreSnapshot{
		putCalls:    s.putCalls,
		putBatches:  s.putBatches,
		putExisted:  s.putExisted,
		putBytes:    s.putBytes,
		getCalls:    s.getCalls,
		getBytes:    s.getBytes,
		existsCalls: s.existsCalls,
		existsHits:  s.existsHits,
		readByRef:   readByRef,
	}
}

func chunkRefStrings(chunks []*blob.Chunk) []string {
	refs := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		refs = append(refs, chunk.GetDataRef().MarshalString())
	}
	return refs
}

func countSamePrefix(before, after []string) int {
	limit := min(len(before), len(after))
	var same int
	for i := range limit {
		if before[i] == after[i] {
			same++
		}
	}
	return same
}

func countChunkFetches(snapshot metricStoreSnapshot, chunks []*blob.Chunk) int {
	var fetches int
	for _, chunk := range chunks {
		fetches += snapshot.readByRef[chunk.GetDataRef().MarshalString()]
	}
	return fetches
}

func countRangeBlobRefs(ranges []*Range) int {
	var count int
	for _, rng := range ranges {
		if rng.GetRef() != nil {
			count++
		}
	}
	return count
}

func syntheticFlatChunkBlob(t *testing.T, chunkCount int, chunkSize uint64) *blob.Blob {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte("synthetic-flat-chunk"), &block.PutOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	chunks := make([]*blob.Chunk, 0, chunkCount)
	var start uint64
	for range chunkCount {
		chunks = append(chunks, blob.NewChunk(ref, chunkSize, start))
		start += chunkSize
	}
	return &blob.Blob{
		BlobType:  blob.BlobType_BlobType_CHUNKED,
		TotalSize: start,
		ChunkIndex: &blob.ChunkIndex{
			Chunks: chunks,
			ChunkerArgs: &blob.ChunkerArgs{
				ChunkerType: blob.ChunkerType_ChunkerType_JC,
			},
		},
	}
}

func readFileBytes(t *testing.T, ctx context.Context, bcs *block.Cursor, root *File) []byte {
	t.Helper()
	rdr := NewHandle(ctx, bcs, root)
	defer rdr.Close()
	out, err := io.ReadAll(rdr)
	if err != nil {
		t.Fatal(err.Error())
	}
	return out
}

func buildMetricRoot(t *testing.T, ctx context.Context, name string, data []byte) (*block.Transaction, *block.Cursor, *File) {
	t.Helper()
	bkt := bucket_mock.NewMockBucket(name, nil)
	return buildMetricRootWithStore(t, ctx, data, bkt, nil)
}

func buildMetricRootWithStore(
	t *testing.T,
	ctx context.Context,
	data []byte,
	store block.StoreOps,
	recorder *metricRecorder,
) (*block.Transaction, *block.Cursor, *File) {
	t.Helper()
	if recorder != nil {
		ctx = blob.WithMetricsRecorder(ctx, recorder)
	}
	btx, bcs := block.NewTransaction(store, nil, nil, nil)
	root := &File{}
	bcs.SetBlock(root, true)
	fh := NewHandle(ctx, bcs, root)
	defer fh.Close()
	fw := NewWriter(fh, btx, metricBlobOpts())
	if err := fw.WriteFrom(0, int64(len(data)), bytes.NewReader(data)); err != nil {
		t.Fatal(err.Error())
	}
	return btx, bcs, root
}

func TestFileMetricAppendHeavyLargeFile(t *testing.T) {
	ctx := context.Background()
	body := bytes.Repeat([]byte("append-metric-base-"), 512)
	appendData := bytes.Repeat([]byte("tail-"), 80)
	store := newMetricCountingStore()
	recorder := &metricRecorder{}
	allocs := testing.AllocsPerRun(1, func() {
		_, _, _ = buildMetricRoot(t, ctx, "test-file-metric-append-alloc", body)
	})
	btx, bcs, root := buildMetricRootWithStore(t, ctx, body, store, recorder)
	initialChunks := root.GetRootBlob().GetChunkIndex().GetChunks()
	if len(initialChunks) < 3 {
		t.Fatalf("expected chunked baseline with prefix chunks, got %d chunks", len(initialChunks))
	}
	prefixRefs := chunkRefStrings(initialChunks[:len(initialChunks)-1])

	store.reset()
	recorder.reset()
	fh := NewHandle(blob.WithMetricsRecorder(ctx, recorder), bcs, root)
	defer fh.Close()
	fw := NewWriter(fh, btx, metricBlobOpts())
	if err := fw.WriteFrom(uint64(len(body)), int64(len(appendData)), bytes.NewReader(appendData)); err != nil {
		t.Fatal(err.Error())
	}
	if len(root.GetRanges()) != 0 {
		t.Fatalf("append created ranges on whole-file root: %d", len(root.GetRanges()))
	}
	publishStarted := time.Now()
	if _, _, err := btx.Write(ctx, true); err != nil {
		t.Fatal(err.Error())
	}
	publishLatency := time.Since(publishStarted)
	nextChunks := root.GetRootBlob().GetChunkIndex().GetChunks()
	preserved := countSamePrefix(prefixRefs, chunkRefStrings(nextChunks[:min(len(prefixRefs), len(nextChunks))]))
	if preserved != len(prefixRefs) {
		t.Fatalf("append rewrote stable prefix refs: preserved %d of %d", preserved, len(prefixRefs))
	}
	out := readFileBytes(t, ctx, bcs, root)
	expected := append(append([]byte(nil), body...), appendData...)
	if !bytes.Equal(out, expected) {
		t.Fatal("append workload readback mismatch")
	}
	metadataBytes, err := root.GetRootBlob().MarshalBlock()
	if err != nil {
		t.Fatal(err.Error())
	}
	directChunks, directBytes := recorder.stage("chunk-direct-put")
	stats := store.snapshot()
	t.Logf("metric workload=append-heavy-large-file old_prefix_refs=%d preserved_prefix_refs=%d range_count=%d metadata_bytes=%d direct_put_chunks=%d direct_put_bytes=%d store_puts=%d duplicate_store_puts=%d store_exists_checks=%d store_exists_hits=%d publish_latency_ns=%d build_allocs_per_run=%.0f", len(prefixRefs), preserved, len(root.GetRanges()), len(metadataBytes), directChunks, directBytes, stats.putCalls, stats.putExisted, stats.existsCalls, stats.existsHits, publishLatency.Nanoseconds(), allocs)
}

func TestFileMetricCompatibleLastRangeAppend(t *testing.T) {
	ctx := context.Background()
	store := newMetricCountingStore()
	body := bytes.Repeat([]byte("range-append-base-"), 96)
	appendData := bytes.Repeat([]byte("tail-range-"), 24)
	btx, bcs, root := buildMetricRootWithStore(t, ctx, body, store, nil)
	fh := NewHandle(ctx, bcs, root)
	defer fh.Close()
	fw := NewWriter(fh, btx, metricBlobOpts())
	if err := fw.WriteFrom(13, int64(len("overwrite")), bytes.NewReader([]byte("overwrite"))); err != nil {
		t.Fatal(err.Error())
	}
	beforeRanges := len(root.GetRanges())
	beforeRefs := countRangeBlobRefs(root.GetRanges())
	if beforeRanges == 0 {
		t.Fatal("expected compatible append fixture to move the file into ranges")
	}

	store.reset()
	if err := fw.WriteFrom(root.GetTotalSize(), int64(len(appendData)), bytes.NewReader(appendData)); err != nil {
		t.Fatal(err.Error())
	}
	if len(root.GetRanges()) != beforeRanges {
		t.Fatalf("compatible EOF append changed range shape: before=%d after=%d", beforeRanges, len(root.GetRanges()))
	}
	if _, _, err := btx.Write(ctx, true); err != nil {
		t.Fatal(err.Error())
	}
	out := readFileBytes(t, ctx, bcs, root)
	expected := append([]byte(nil), body...)
	copy(expected[13:], []byte("overwrite"))
	expected = append(expected, appendData...)
	if !bytes.Equal(out, expected) {
		t.Fatal("compatible last-range append readback mismatch")
	}
	afterRefs := countRangeBlobRefs(root.GetRanges())
	if afterRefs-beforeRefs > 2 {
		t.Fatalf("compatible EOF append widened range blob refs by %d, want <=2", afterRefs-beforeRefs)
	}
	stats := store.snapshot()
	t.Logf("metric workload=compatible-last-range-append before_ranges=%d after_ranges=%d before_block_refs=%d after_block_refs=%d store_puts=%d duplicate_store_puts=%d put_bytes=%d", beforeRanges, len(root.GetRanges()), beforeRefs, afterRefs, stats.putCalls, stats.putExisted, stats.putBytes)
}

func TestFileMetricRandomOverwrite(t *testing.T) {
	ctx := context.Background()
	body := bytes.Repeat([]byte("random-overwrite-base-"), 384)
	store := newMetricCountingStore()
	btx, bcs, root := buildMetricRootWithStore(t, ctx, body, store, nil)
	expected := append([]byte(nil), body...)
	writes := []struct {
		offset int
		data   []byte
	}{
		{96, bytes.Repeat([]byte("a"), 64)},
		{96, bytes.Repeat([]byte("b"), 64)},
		{220, bytes.Repeat([]byte("c"), 48)},
		{300, bytes.Repeat([]byte("d"), 24)},
	}
	fh := NewHandle(ctx, bcs, root)
	defer fh.Close()
	fw := NewWriter(fh, btx, nil)
	for _, write := range writes {
		copy(expected[write.offset:], write.data)
		if err := fw.WriteFrom(uint64(write.offset), int64(len(write.data)), bytes.NewReader(write.data)); err != nil {
			t.Fatal(err.Error())
		}
	}
	publishStarted := time.Now()
	rootRef, bcs, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	publishLatency := time.Since(publishStarted)

	store.reset()
	openStarted := time.Now()
	_, readBcs := block.NewTransaction(store, nil, rootRef, nil)
	root, err = block.UnmarshalBlock[*File](ctx, readBcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	openLatency := time.Since(openStarted)
	openStats := store.snapshot()
	store.reset()
	if got := readFileBytes(t, ctx, readBcs, root); !bytes.Equal(got, expected) {
		t.Fatal("random overwrite readback changed after compaction")
	}

	occluded := countFullyOccludedRanges(root.GetRanges())
	overlapDepth := maxOverlapDepth(root.GetRanges())
	lookupScan := lookupScanLength(root.GetRanges(), 128)
	uncompactedRangeCount := 1 + len(writes)
	if len(root.GetRanges()) >= uncompactedRangeCount || occluded != 0 || overlapDepth <= 1 {
		t.Fatalf("random overwrite compaction did not reduce stale range pressure: ranges=%d uncompacted_ranges=%d occluded=%d overlap_depth=%d", len(root.GetRanges()), uncompactedRangeCount, occluded, overlapDepth)
	}
	_ = bcs
	t.Logf("metric workload=random-overwrite range_count=%d uncompacted_range_count=%d fully_occluded_range_count=%d stale_reachable_refs=%d overlap_depth=%d lookup_scan_length=%d logical_bytes=%d root_open_latency_ns=%d publish_latency_ns=%d root_fetches=%d fetched_bytes=%d", len(root.GetRanges()), uncompactedRangeCount, occluded, occluded, overlapDepth, lookupScan, root.GetTotalSize(), openLatency.Nanoseconds(), publishLatency.Nanoseconds(), openStats.getCalls, openStats.getBytes)
}

func TestFileRangeCompactionPreservesSparseZeroOverwrite(t *testing.T) {
	ctx := context.Background()
	body := []byte("0123456789abcdef")
	btx, bcs, root := buildMetricRoot(t, ctx, "range-compaction-sparse-zero", body)
	fh := NewHandle(ctx, bcs, root)
	defer fh.Close()
	fw := NewWriter(fh, btx, nil)
	if err := fw.WriteFrom(4, 4, bytes.NewReader([]byte("DATA"))); err != nil {
		t.Fatal(err.Error())
	}
	if err := fw.WriteBlob(4, 4, nil); err != nil {
		t.Fatal(err.Error())
	}

	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, readBcs := block.NewTransaction(btx.GetStoreOps(), nil, rootRef, nil)
	readRoot, err := block.UnmarshalBlock[*File](ctx, readBcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	got := readFileBytes(t, ctx, readBcs, readRoot)
	want := append([]byte(nil), body...)
	for i := range want[4:8] {
		want[4+i] = 0
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("sparse zero overwrite readback mismatch\n got: %v\nwant: %v", got, want)
	}
	if len(readRoot.GetRanges()) != 2 {
		t.Fatalf("expected compacted root range and sparse zero range, got %d ranges", len(readRoot.GetRanges()))
	}
	if countFullyOccludedRanges(readRoot.GetRanges()) != 0 {
		t.Fatalf("expected no fully occluded ranges after compaction, got %d", countFullyOccludedRanges(readRoot.GetRanges()))
	}
	if refs := countRangeBlobRefs(readRoot.GetRanges()); refs != 1 {
		t.Fatalf("expected stale concrete overwrite ref to be dropped, got %d range blob refs", refs)
	}
}

func TestFileMetricOverlappingRangeReadback(t *testing.T) {
	ctx := context.Background()
	store := newMetricCountingStore()
	body := bytes.Repeat([]byte("b"), 32)
	btx, bcs, root := buildMetricRootWithStore(t, ctx, body, store, nil)
	fh := NewHandle(ctx, bcs, root)
	defer fh.Close()
	fw := NewWriter(fh, btx, metricBlobOpts())
	if err := fw.WriteFrom(8, 20, bytes.NewReader(bytes.Repeat([]byte("l"), 20))); err != nil {
		t.Fatal(err.Error())
	}
	if err := fw.WriteFrom(14, 2, bytes.NewReader([]byte("xx"))); err != nil {
		t.Fatal(err.Error())
	}
	if err := fw.WriteFrom(12, 4, bytes.NewReader([]byte("HIGH"))); err != nil {
		t.Fatal(err.Error())
	}
	if err := fw.WriteBlob(20, 4, nil); err != nil {
		t.Fatal(err.Error())
	}

	publishStarted := time.Now()
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	publishLatency := time.Since(publishStarted)

	store.reset()
	openStarted := time.Now()
	_, readBcs := block.NewTransaction(store, nil, rootRef, nil)
	readRoot, err := block.UnmarshalBlock[*File](ctx, readBcs, NewFileBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	openLatency := time.Since(openStarted)
	openStats := store.snapshot()
	store.reset()

	rdr := NewHandle(ctx, readBcs, readRoot)
	defer rdr.Close()
	if _, seekErr := rdr.Seek(12, io.SeekStart); seekErr != nil {
		t.Fatal(seekErr.Error())
	}
	got := make([]byte, 20)
	readStarted := time.Now()
	n, err := rdr.Read(got)
	readLatency := time.Since(readStarted)
	if err != nil {
		t.Fatal(err.Error())
	}
	if n != len(got) {
		t.Fatalf("overlap metric read returned %d bytes, expected %d", n, len(got))
	}
	want := append([]byte("HIGH"), bytes.Repeat([]byte("l"), 4)...)
	want = append(want, 0, 0, 0, 0)
	want = append(want, bytes.Repeat([]byte("l"), 4)...)
	want = append(want, bytes.Repeat([]byte("b"), 4)...)
	if !bytes.Equal(got, want) {
		t.Fatal("overlap metric readback mismatch")
	}

	ranges := readRoot.GetRanges()
	rangeCount := len(ranges)
	occluded := countFullyOccludedRanges(ranges)
	overlapDepth := maxOverlapDepth(ranges)
	lookupScan := lookupScanLength(ranges, 14)
	if rangeCount <= 1 || occluded != 0 || overlapDepth <= 1 {
		t.Fatalf("overlap workload compaction did not preserve visible overlap without stale ranges: ranges=%d occluded=%d overlap_depth=%d", rangeCount, occluded, overlapDepth)
	}
	stats := store.snapshot()
	t.Logf("metric workload=overlap-readback-corrected range_count=%d fully_occluded_range_count=%d stale_reachable_refs=%d overlap_depth=%d lookup_scan_length=%d read_latency_ns=%d read_latency_by_range_count_ns=%d logical_bytes=%d root_open_latency_ns=%d publish_latency_ns=%d root_open_fetches=%d root_open_fetched_bytes=%d read_fetches=%d read_fetched_bytes=%d", rangeCount, occluded, occluded, overlapDepth, lookupScan, readLatency.Nanoseconds(), readLatency.Nanoseconds()/int64(max(rangeCount, 1)), readRoot.GetTotalSize(), openLatency.Nanoseconds(), publishLatency.Nanoseconds(), openStats.getCalls, openStats.getBytes, stats.getCalls, stats.getBytes)
}

func TestFileMetricMostlyUnchangedFullRewrite(t *testing.T) {
	ctx := context.Background()
	body := bytes.Repeat([]byte("mostly-unchanged-full-rewrite-"), 384)
	store := newMetricCountingStore()
	recorder := &metricRecorder{}
	btx, bcs, root := buildMetricRootWithStore(t, ctx, body, store, recorder)
	oldRefs := chunkRefStrings(root.GetRootBlob().GetChunkIndex().GetChunks())
	mutated := append([]byte(nil), body...)
	mutated[len(mutated)/2] ^= 0xff
	store.reset()
	recorder.reset()
	fh := NewHandle(blob.WithMetricsRecorder(ctx, recorder), bcs, root)
	defer fh.Close()
	fw := NewWriter(fh, btx, metricBlobOpts())
	if err := fw.WriteFrom(0, int64(len(mutated)), bytes.NewReader(mutated)); err != nil {
		t.Fatal(err.Error())
	}
	publishStarted := time.Now()
	if _, _, err := btx.Write(ctx, true); err != nil {
		t.Fatal(err.Error())
	}
	publishLatency := time.Since(publishStarted)
	out := readFileBytes(t, ctx, bcs, root)
	if !bytes.Equal(out, mutated) {
		t.Fatal("full rewrite workload readback mismatch")
	}
	newRefs := chunkRefStrings(root.GetRootBlob().GetChunkIndex().GetChunks())
	unchangedPositions := countSamePrefix(oldRefs, newRefs)
	directChunks, directBytes := recorder.stage("chunk-direct-put")
	stats := store.snapshot()
	t.Logf("metric workload=mostly-unchanged-full-rewrite old_chunks=%d new_chunks=%d unchanged_ref_positions=%d reencoded_chunk_positions=%d direct_put_chunks=%d direct_put_bytes=%d duplicate_store_puts=%d store_puts=%d store_exists_hits=%d publish_latency_ns=%d", len(oldRefs), len(newRefs), unchangedPositions, len(newRefs)-unchangedPositions, directChunks, directBytes, stats.putExisted, stats.putCalls, stats.existsHits, publishLatency.Nanoseconds())
}

func TestFileMetricSequentialOpenDownload(t *testing.T) {
	ctx := context.Background()
	first := bytes.Repeat([]byte("download-a-"), 384)
	second := bytes.Repeat([]byte("download-b-"), 384)
	store := newMetricCountingStore()
	btx, bcs, root := buildMetricRootWithStore(t, ctx, first, store, nil)
	fh := NewHandle(ctx, bcs, root)
	fw := NewWriter(fh, btx, metricBlobOpts())
	if err := fw.WriteFrom(uint64(len(first)), int64(len(second)), bytes.NewReader(second)); err != nil {
		t.Fatal(err.Error())
	}
	fh.Close()
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, bufSize := range []int{37, 257} {
		store.reset()
		_, readBcs := block.NewTransaction(store, nil, rootRef, nil)
		readRoot, err := block.UnmarshalBlock[*File](ctx, readBcs, NewFileBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		buf := make([]byte, bufSize)
		rdr := NewHandle(ctx, readBcs, readRoot)
		var total, reads int
		readStarted := time.Now()
		for {
			n, err := rdr.Read(buf)
			if n > 0 {
				total += n
				reads++
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				rdr.Close()
				t.Fatal(err.Error())
			}
		}
		if err := rdr.Close(); err != nil {
			t.Fatal(err.Error())
		}
		if total != len(first)+len(second) {
			t.Fatalf("sequential read got %d bytes, expected %d", total, len(first)+len(second))
		}
		stats := store.snapshot()
		chunkFetches := countChunkFetches(stats, readRoot.GetRootBlob().GetChunkIndex().GetChunks())
		cacheHits := max(reads-chunkFetches, 0)
		rangeTransitions := len(readRoot.GetRanges())
		blobOpens := 1
		if rangeTransitions > 0 {
			blobOpens = countRangeBlobRefs(readRoot.GetRanges())
		}
		t.Logf("metric workload=open-download-sequential buffer_bytes=%d read_calls=%d root_opens=1 range_transitions=%d blob_opens=%d chunk_fetches=%d current_chunk_cache_hits=%d fetched_bytes=%d read_latency_ns=%d retained_chunk_bytes_bound=%d", bufSize, reads, rangeTransitions, blobOpens, chunkFetches, cacheHits, stats.getBytes, time.Since(readStarted).Nanoseconds(), bufSize+128)
	}
}

func TestFileMetricChunkMetadataOverhead(t *testing.T) {
	ctx := context.Background()
	const chunkSize = uint64(1024)
	for _, chunkCount := range []int{4096, 40960} {
		store := newMetricCountingStore()
		rootBlob := syntheticFlatChunkBlob(t, chunkCount, chunkSize)
		root := &File{
			TotalSize: rootBlob.GetTotalSize(),
			RootBlob:  rootBlob,
		}
		metadataBytes, err := rootBlob.MarshalBlock()
		if err != nil {
			t.Fatal(err.Error())
		}
		appendedBlob := syntheticFlatChunkBlob(t, chunkCount+1, chunkSize)
		appendedMetadataBytes, err := appendedBlob.MarshalBlock()
		if err != nil {
			t.Fatal(err.Error())
		}
		btx, bcs := block.NewTransaction(store, nil, nil, nil)
		bcs.SetBlock(root, true)
		writeStarted := time.Now()
		if _, _, err := btx.Write(ctx, true); err != nil {
			t.Fatal(err.Error())
		}
		rootWriteLatency := time.Since(writeStarted)
		stats := store.snapshot()
		t.Logf("metric workload=chunk-metadata-overhead file_class=synthetic-flat chunk_class=target-%d file_bytes=%d chunk_count=%d serialized_metadata_bytes=%d metadata_rewrite_bytes_per_append=%d metadata_rewrite_bytes_per_publish=%d root_write_latency_ns=%d store_puts=%d put_bytes=%d", chunkCount, rootBlob.GetTotalSize(), len(rootBlob.GetChunkIndex().GetChunks()), len(metadataBytes), len(appendedMetadataBytes), len(metadataBytes), rootWriteLatency.Nanoseconds(), stats.putCalls, stats.putBytes)
	}
}

func countFullyOccludedRanges(ranges []*Range) int {
	var count int
	for i, rng := range ranges {
		if rangeFullyOccluded(i, ranges, rng) {
			count++
		}
	}
	return count
}

func rangeFullyOccluded(idx int, ranges []*Range, rng *Range) bool {
	start := rng.GetStart()
	end := start + rng.GetLength()
	for pos := start; pos < end; pos++ {
		covered := false
		for j, other := range ranges {
			if j == idx || other.GetNonce() <= rng.GetNonce() {
				continue
			}
			if other.GetStart() <= pos && pos < other.GetStart()+other.GetLength() {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func maxOverlapDepth(ranges []*Range) int {
	var maxDepth int
	for _, rng := range ranges {
		end := rng.GetStart() + rng.GetLength()
		for pos := rng.GetStart(); pos < end; pos++ {
			var depth int
			for _, other := range ranges {
				if other.GetStart() <= pos && pos < other.GetStart()+other.GetLength() {
					depth++
				}
			}
			if depth > maxDepth {
				maxDepth = depth
			}
		}
	}
	return maxDepth
}

func lookupScanLength(ranges []*Range, pos uint64) int {
	idxAfter := len(ranges)
	for i, rng := range ranges {
		if rng.GetStart() > pos {
			idxAfter = i
			break
		}
	}
	var scans int
	for i := idxAfter - 1; i >= 0; i-- {
		scans++
		rng := ranges[i]
		start := rng.GetStart()
		end := start + rng.GetLength()
		if start <= pos && pos < end {
			continue
		}
	}
	return scans
}
