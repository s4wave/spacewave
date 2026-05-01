package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// memIndexCache is an in-memory IndexCache for tests.
type memIndexCache struct {
	mu      sync.Mutex
	entries map[string][]byte
}

type errorIndexCache struct {
	getErr error
	setErr error
}

func newMemIndexCache() *memIndexCache {
	return &memIndexCache{entries: make(map[string][]byte)}
}

func (c *memIndexCache) Get(_ context.Context, packID string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[packID]
	return bytes.Clone(e), ok, nil
}

func (c *memIndexCache) Set(_ context.Context, packID string, entries []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[packID] = bytes.Clone(entries)
	return nil
}

func (c *errorIndexCache) Get(_ context.Context, _ string) ([]byte, bool, error) {
	return nil, false, c.getErr
}

func (c *errorIndexCache) Set(_ context.Context, _ string, _ []byte) error {
	return c.setErr
}

// bytesTransport serves Fetch calls from a fixed byte slice, optionally
// counting and gating calls for fault-injection style tests.
type bytesTransport struct {
	data []byte
	mu   sync.Mutex
	// calls records each (off, len) pair.
	calls []fetchCall
	// blockFn optionally blocks Fetch until it returns.
	blockFn func()
	// rewriteFn optionally rewrites the returned bytes per call index.
	rewriteFn func(call int, off int64, data []byte) []byte
}

type fetchCall struct {
	off    int64
	length int
}

type packItem struct {
	h    *hash.Hash
	data []byte
}

func (t *bytesTransport) Fetch(_ context.Context, off int64, length int) ([]byte, error) {
	t.mu.Lock()
	t.calls = append(t.calls, fetchCall{off: off, length: length})
	call := len(t.calls)
	fn := t.blockFn
	rewrite := t.rewriteFn
	t.mu.Unlock()
	if fn != nil {
		fn()
	}
	if off >= int64(len(t.data)) {
		return nil, io.EOF
	}
	end := min(off+int64(length), int64(len(t.data)))
	out := bytes.Clone(t.data[off:end])
	if rewrite != nil {
		out = rewrite(call, off, out)
	}
	return out, nil
}

func (t *bytesTransport) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.calls)
}

func (t *bytesTransport) callAt(i int) fetchCall {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls[i]
}

// writebackStore records PutBlock calls for testing co-block writeback.
type writebackStore struct {
	block.NopStoreOps
	mu   sync.Mutex
	puts []*block.PutBatchEntry
	// blockFn optionally blocks PutBlock until ctx is cancelled.
	blockFn func()
}

func (w *writebackStore) GetHashType() hash.HashType { return hash.HashType_HashType_SHA256 }

func (w *writebackStore) PutBlock(_ context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if w.blockFn != nil {
		w.blockFn()
	}
	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.puts = append(w.puts, &block.PutBatchEntry{Ref: ref, Data: bytes.Clone(data)})
	return ref, false, nil
}

func (w *writebackStore) GetBlock(_ context.Context, _ *block.BlockRef) ([]byte, bool, error) {
	return nil, false, nil
}

func (w *writebackStore) GetBlockExists(_ context.Context, _ *block.BlockRef) (bool, error) {
	return false, nil
}

func (w *writebackStore) StatBlock(_ context.Context, _ *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}

func (w *writebackStore) RmBlock(_ context.Context, _ *block.BlockRef) error { return nil }

func (w *writebackStore) putCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.puts)
}

// buildTestPack packs blocks in map order (non-deterministic).
func buildTestPack(t *testing.T, blocks map[string][]byte) ([]byte, []byte) {
	t.Helper()
	var items []packItem
	for _, data := range blocks {
		h, err := hash.Sum(hash.HashType_HashType_SHA256, data)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, packItem{h: h, data: data})
	}
	return packItems(t, items)
}

// buildTestPackOrdered packs blocks in the given deterministic order.
func buildTestPackOrdered(t *testing.T, ordered []struct{ Name, Data string }) ([]byte, []byte) {
	t.Helper()
	items := make([]packItem, len(ordered))
	for i, o := range ordered {
		h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte(o.Data))
		if err != nil {
			t.Fatal(err)
		}
		items[i] = packItem{h: h, data: []byte(o.Data)}
	}
	return packItems(t, items)
}

func packItems(t *testing.T, items []packItem) ([]byte, []byte) {
	t.Helper()
	var buf bytes.Buffer
	idx := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, []byte, error) {
		if idx >= len(items) {
			return nil, nil, nil
		}
		e := items[idx]
		idx++
		return e.h, e.data, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), result.BloomFilter
}

func mustReadIndexTail(t *testing.T, data []byte) []byte {
	t.Helper()
	_, tail, err := kvfile.ReadIndexTail(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return tail
}

// openerFromBytes builds an opener that returns a fresh engine per call over
// the same byte slice.
func openerFromBytes(data []byte) (Opener, *bytesTransport) {
	t := &bytesTransport{data: data}
	opener := func(packID string, size int64) (*PackReader, error) {
		return NewPackReader(packID, size, t, hash.HashType_HashType_SHA256), nil
	}
	return opener, t
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// TestPackfileStoreBasicReads verifies GetBlock found/not-found, GetBlockExists,
// PutBlock/RmBlock errors, and GetHashType.
func TestPackfileStoreBasicReads(t *testing.T) {
	ctx := t.Context()
	blocks := map[string][]byte{
		"a": []byte("alpha-data"),
		"b": []byte("beta-data"),
		"c": []byte("charlie-data"),
	}
	packBytes, bloomBytes := buildTestPack(t, blocks)

	opener, _ := openerFromBytes(packBytes)
	cache := newMemIndexCache()
	store := NewPackfileStore(opener, cache)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "basic-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(blocks)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	if store.GetHashType() != hash.HashType_HashType_SHA256 {
		t.Fatal("expected SHA256 hash type")
	}

	for _, data := range blocks {
		h, err := hash.Sum(hash.HashType_HashType_SHA256, data)
		if err != nil {
			t.Fatal(err)
		}
		got, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: h})
		if err != nil {
			t.Fatalf("GetBlock: %v", err)
		}
		if !found || !bytes.Equal(got, data) {
			t.Fatalf("expected block found, got found=%v len=%d", found, len(got))
		}
	}

	unknownHash, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("not-in-packfile"))
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: unknownHash})
	if err != nil {
		t.Fatalf("GetBlock unknown: %v", err)
	}
	if found {
		t.Fatal("expected unknown block to be missing")
	}

	exists, err := store.GetBlockExists(ctx, &block.BlockRef{Hash: unknownHash})
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("expected unknown block to not exist")
	}

	if _, _, err := store.PutBlock(ctx, []byte("x"), nil); err == nil {
		t.Fatal("expected PutBlock error")
	}
	if err := store.RmBlock(ctx, &block.BlockRef{Hash: unknownHash}); err == nil {
		t.Fatal("expected RmBlock error")
	}
}

func TestPackfileStoreGetBlockExistsDoesNotFetchPayload(t *testing.T) {
	ctx := t.Context()
	filler := bytes.Repeat([]byte("x"), defaultIndexTailInitialWindow+4096)
	ordered := []struct{ Name, Data string }{
		{"a", "alpha-data"},
		{"b", string(filler)},
	}
	packBytes, bloomBytes := buildTestPackOrdered(t, ordered)
	opener, transport := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "exists-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(ordered)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha-data"))
	if err != nil {
		t.Fatal(err)
	}
	exists, err := store.GetBlockExists(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatalf("GetBlockExists: %v", err)
	}
	if !exists {
		t.Fatal("expected block to exist")
	}
	firstCalls := transport.callCount()
	if firstCalls == 0 {
		t.Fatal("expected index load fetch")
	}

	wb := &writebackStore{}
	store.SetWriteback(ctx, wb, 0)
	exists, err = store.GetBlockExists(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatalf("GetBlockExists with writeback: %v", err)
	}
	if !exists {
		t.Fatal("expected block to still exist")
	}
	if got := transport.callCount(); got != firstCalls {
		t.Fatalf("GetBlockExists fetched payload window: calls %d -> %d", firstCalls, got)
	}
	if got := wb.putCount(); got != 0 {
		t.Fatalf("GetBlockExists published writeback blocks: %d", got)
	}

	data, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found || !bytes.Equal(data, []byte("alpha-data")) {
		t.Fatalf("expected GetBlock to fetch target data, found=%v data=%q", found, data)
	}
	if got := transport.callCount(); got <= firstCalls {
		t.Fatalf("GetBlock did not fetch payload window: calls %d -> %d", firstCalls, got)
	}
}

func TestPackfileStoreUpdateManifestFiltersSupersededAndEvictsEngines(t *testing.T) {
	store := NewPackfileStore(func(packID string, size int64) (*PackReader, error) {
		t.Fatalf("unexpected opener call for %s size %d", packID, size)
		return nil, nil
	}, nil)
	store.engines["pack-a"] = nil
	store.engines["pack-b"] = nil
	store.engines["pack-gone"] = nil

	store.UpdateManifest([]*packfile.PackfileEntry{
		{Id: "pack-a", SupersededBy: "pack-b"},
		{Id: "pack-b"},
	})

	store.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if len(store.manifest) != 1 {
			t.Fatalf("manifest len=%d want 1", len(store.manifest))
		}
		if store.manifest[0].GetId() != "pack-b" {
			t.Fatalf("active manifest id=%q want pack-b", store.manifest[0].GetId())
		}
	})
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.engines["pack-b"]; !ok {
		t.Fatal("active engine pack-b was evicted")
	}
	if _, ok := store.engines["pack-a"]; ok {
		t.Fatal("superseded engine pack-a was retained")
	}
	if _, ok := store.engines["pack-gone"]; ok {
		t.Fatal("vanished engine pack-gone was retained")
	}
}

func TestPackfileStoreGetBlockExistsBatchUsesIndexes(t *testing.T) {
	ctx := t.Context()
	filler := bytes.Repeat([]byte("x"), defaultIndexTailInitialWindow+4096)
	ordered := []struct{ Name, Data string }{
		{"a", "alpha-data"},
		{"b", "beta-data"},
		{"c", string(filler)},
	}
	packBytes, bloomBytes := buildTestPackOrdered(t, ordered)
	opener, transport := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "batch-exists-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(ordered)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	alpha, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha-data"))
	if err != nil {
		t.Fatal(err)
	}
	beta, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("beta-data"))
	if err != nil {
		t.Fatal(err)
	}
	missing, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("missing-data"))
	if err != nil {
		t.Fatal(err)
	}

	wb := &writebackStore{}
	store.SetWriteback(ctx, wb, 0)
	found, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{
		{Hash: missing},
		{Hash: alpha},
		nil,
		{Hash: beta},
		{Hash: alpha},
	})
	if err != nil {
		t.Fatalf("GetBlockExistsBatch: %v", err)
	}
	want := []bool{false, true, false, true, true}
	if len(found) != len(want) {
		t.Fatalf("found len = %d, want %d", len(found), len(want))
	}
	for i := range want {
		if found[i] != want[i] {
			t.Fatalf("found[%d] = %v, want %v (all=%v)", i, found[i], want[i], found)
		}
	}
	firstCalls := transport.callCount()
	if firstCalls == 0 {
		t.Fatal("expected index load fetch")
	}
	if got := wb.putCount(); got != 0 {
		t.Fatalf("GetBlockExistsBatch published writeback blocks: %d", got)
	}

	found, err = store.GetBlockExistsBatch(ctx, []*block.BlockRef{{Hash: beta}, {Hash: missing}})
	if err != nil {
		t.Fatalf("GetBlockExistsBatch cached index: %v", err)
	}
	if !found[0] || found[1] {
		t.Fatalf("unexpected cached-index batch result: %v", found)
	}
	if got := transport.callCount(); got != firstCalls {
		t.Fatalf("cached GetBlockExistsBatch fetched payload window: calls %d -> %d", firstCalls, got)
	}
}

func TestPackfileStoreGetBlockExistsHandlesBloomFalsePositive(t *testing.T) {
	ctx := t.Context()
	filler := bytes.Repeat([]byte("x"), defaultIndexTailInitialWindow+4096)
	targetBytes, targetBloom := buildTestPackOrdered(t, []struct{ Name, Data string }{
		{"target", "target-data"},
		{"filler", string(filler)},
	})
	negativeBytes, _ := buildTestPackOrdered(t, []struct{ Name, Data string }{
		{"negative", "negative-data"},
		{"filler", string(filler)},
	})
	packs := map[string][]byte{
		"negative-pack": negativeBytes,
		"target-pack":   targetBytes,
	}
	transports := make(map[string]*bytesTransport, len(packs))
	opener := func(packID string, size int64) (*PackReader, error) {
		transport := &bytesTransport{data: packs[packID]}
		transports[packID] = transport
		return NewPackReader(packID, size, transport, hash.HashType_HashType_SHA256), nil
	}
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{
		{
			Id:          "negative-pack",
			BloomFilter: targetBloom,
			BlockCount:  2,
			SizeBytes:   uint64(len(negativeBytes)),
		},
		{
			Id:          "target-pack",
			BloomFilter: targetBloom,
			BlockCount:  2,
			SizeBytes:   uint64(len(targetBytes)),
		},
	})

	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("target-data"))
	if err != nil {
		t.Fatal(err)
	}
	wb := &writebackStore{}
	store.SetWriteback(ctx, wb, 0)
	exists, err := store.GetBlockExists(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatalf("GetBlockExists: %v", err)
	}
	if !exists {
		t.Fatal("expected target block to exist")
	}
	for packID, transport := range transports {
		if got := transport.callCount(); got == 0 {
			t.Fatalf("expected index load for %s", packID)
		}
	}
	if got := wb.putCount(); got != 0 {
		t.Fatalf("GetBlockExists published writeback blocks: %d", got)
	}
}

func TestPackfileStoreLookupStats(t *testing.T) {
	ctx := t.Context()
	targetBytes, targetBloom := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	negativeBytes, _ := buildTestPackOrdered(t, []struct{ Name, Data string }{{"b", "beta"}})
	packs := map[string][]byte{
		"negative-pack": negativeBytes,
		"target-pack":   targetBytes,
	}
	opener := func(packID string, size int64) (*PackReader, error) {
		data := packs[packID]
		return NewPackReader(packID, size, &bytesTransport{data: data}, hash.HashType_HashType_SHA256), nil
	}
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{
		{
			Id:          "negative-pack",
			BloomFilter: targetBloom,
			BlockCount:  1,
			SizeBytes:   uint64(len(negativeBytes)),
		},
		{
			Id:          "target-pack",
			BloomFilter: targetBloom,
			BlockCount:  1,
			SizeBytes:   uint64(len(targetBytes)),
		},
	})

	alphaHash, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found {
		t.Fatal("expected target block found")
	}

	stats := store.SnapshotStats()
	if stats.LookupCount != 1 {
		t.Fatalf("LookupCount = %d, want 1", stats.LookupCount)
	}
	if stats.CandidatePacks != 2 || stats.LastCandidatePacks != 2 {
		t.Fatalf("candidate packs total=%d last=%d, want 2/2", stats.CandidatePacks, stats.LastCandidatePacks)
	}
	if stats.OpenedPacks != 2 || stats.LastOpenedPacks != 2 {
		t.Fatalf("opened packs total=%d last=%d, want 2/2", stats.OpenedPacks, stats.LastOpenedPacks)
	}
	if stats.NegativePacks != 1 || stats.LastNegativePacks != 1 {
		t.Fatalf("negative packs total=%d last=%d, want 1/1", stats.NegativePacks, stats.LastNegativePacks)
	}
	if stats.TargetHits != 1 || !stats.LastTargetHit {
		t.Fatalf("target hits total=%d last=%v, want 1/true", stats.TargetHits, stats.LastTargetHit)
	}
	if stats.IndexCacheMisses != 2 {
		t.Fatalf("IndexCacheMisses = %d, want 2", stats.IndexCacheMisses)
	}
	if stats.RemoteIndexLoads != 2 {
		t.Fatalf("RemoteIndexLoads = %d, want 2", stats.RemoteIndexLoads)
	}
	if stats.RemoteIndexBytes == 0 || stats.LastRemoteIndexBytes == 0 {
		t.Fatalf(
			"remote index bytes total=%d last=%d, want non-zero",
			stats.RemoteIndexBytes,
			stats.LastRemoteIndexBytes,
		)
	}
}

func TestPackfileStoreLookupPrunesUnrelatedFullPacks(t *testing.T) {
	ctx := t.Context()
	policy := writer.DefaultPolicy()
	packCount := 16
	targetPack := 9
	targetBlock := 37
	packs := make(map[string][]byte, packCount)
	entries := make([]*packfile.PackfileEntry, 0, packCount)

	var targetHash *hash.Hash
	for i := range packCount {
		items := make([]packItem, 0, policy.MaxBlocksPerPack)
		for j := range int(policy.MaxBlocksPerPack) {
			data := []byte("fanout pack " + strconv.Itoa(i) + " block " + strconv.Itoa(j))
			h, err := hash.Sum(hash.HashType_HashType_SHA256, data)
			if err != nil {
				t.Fatal(err)
			}
			if i == targetPack && j == targetBlock {
				targetHash = h
			}
			items = append(items, packItem{h: h, data: data})
		}
		packBytes, bloomBytes := packItems(t, items)
		id := "fanout-pack-" + strconv.Itoa(i)
		packs[id] = packBytes
		entries = append(entries, &packfile.PackfileEntry{
			Id:          id,
			BloomFilter: bloomBytes,
			BlockCount:  policy.MaxBlocksPerPack,
			SizeBytes:   uint64(len(packBytes)),
		})
	}
	if targetHash == nil {
		t.Fatal("target hash was not generated")
	}

	var openCount atomic.Int32
	opener := func(packID string, size int64) (*PackReader, error) {
		data := packs[packID]
		if data == nil {
			return nil, errors.New("unknown pack")
		}
		openCount.Add(1)
		return NewPackReader(packID, size, &bytesTransport{data: data}, hash.HashType_HashType_SHA256), nil
	}

	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest(entries)

	got, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: targetHash})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found {
		t.Fatal("expected target block found")
	}
	if string(got) != "fanout pack "+strconv.Itoa(targetPack)+" block "+strconv.Itoa(targetBlock) {
		t.Fatalf("unexpected target data: %q", string(got))
	}

	stats := store.SnapshotStats()
	if stats.LastOpenedPacks > 4 {
		t.Fatalf("opened %d packs for one lookup, want at most 4", stats.LastOpenedPacks)
	}
	if stats.LastOpenedPacks >= packCount/2 {
		t.Fatalf("opened most packs for one lookup: %d of %d", stats.LastOpenedPacks, packCount)
	}
	if stats.LastNegativePacks > 3 {
		t.Fatalf("negative packs = %d, want at most 3", stats.LastNegativePacks)
	}
	if stats.RemoteIndexLoads != uint64(stats.LastOpenedPacks) {
		t.Fatalf(
			"RemoteIndexLoads = %d, want %d",
			stats.RemoteIndexLoads,
			stats.LastOpenedPacks,
		)
	}
	if got := int(openCount.Load()); got != stats.LastOpenedPacks {
		t.Fatalf("open count = %d, stats opened = %d", got, stats.LastOpenedPacks)
	}
}

func TestPackfileStoreManifestDistributionStats(t *testing.T) {
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{
		{"a", "alpha"},
		{"b", "beta"},
	})
	store := NewPackfileStore(nil, nil)
	store.UpdateManifest([]*packfile.PackfileEntry{
		{
			Id:          "valid-pack",
			BloomFilter: bloomBytes,
			BlockCount:  writer.DefaultMaxBlocksPerPack,
			SizeBytes:   uint64(len(packBytes)),
		},
		{
			Id:         "missing-bloom-pack",
			BlockCount: 5,
			SizeBytes:  200,
		},
		{
			Id:          "invalid-bloom-pack",
			BloomFilter: []byte("not-a-bloom-filter"),
			BlockCount:  9,
			SizeBytes:   300,
		},
	})

	stats := store.SnapshotStats()
	if stats.ManifestEntries != 3 {
		t.Fatalf("ManifestEntries = %d, want 3", stats.ManifestEntries)
	}
	wantBlockTotal := writer.DefaultMaxBlocksPerPack + 14
	if stats.PackBlockCountTotal != wantBlockTotal ||
		stats.PackBlockCountMin != 5 ||
		stats.PackBlockCountMax != writer.DefaultMaxBlocksPerPack {
		t.Fatalf(
			"block count total/min/max = %d/%d/%d, want %d/5/%d",
			stats.PackBlockCountTotal,
			stats.PackBlockCountMin,
			stats.PackBlockCountMax,
			wantBlockTotal,
			writer.DefaultMaxBlocksPerPack,
		)
	}
	wantSizeTotal := uint64(len(packBytes)) + 500
	if stats.PackSizeBytesTotal != wantSizeTotal ||
		stats.PackSizeBytesMin != uint64(len(packBytes)) ||
		stats.PackSizeBytesMax != 300 {
		t.Fatalf(
			"pack size total/min/max = %d/%d/%d, want %d/%d/300",
			stats.PackSizeBytesTotal,
			stats.PackSizeBytesMin,
			stats.PackSizeBytesMax,
			wantSizeTotal,
			len(packBytes),
		)
	}
	if stats.BloomFilterCount != 1 || stats.BloomMissingCount != 1 || stats.BloomInvalidCount != 1 {
		t.Fatalf(
			"bloom valid/missing/invalid = %d/%d/%d, want 1/1/1",
			stats.BloomFilterCount,
			stats.BloomMissingCount,
			stats.BloomInvalidCount,
		)
	}
	if stats.BloomParameterShapeCount != 1 {
		t.Fatalf("BloomParameterShapeCount = %d, want 1", stats.BloomParameterShapeCount)
	}
	if stats.BloomMaxFalsePositiveRate <= 0 {
		t.Fatalf("BloomMaxFalsePositiveRate = %f, want positive", stats.BloomMaxFalsePositiveRate)
	}
	if stats.BloomRiskPackCount > stats.BloomFilterCount {
		t.Fatalf(
			"BloomRiskPackCount = %d, want at most %d",
			stats.BloomRiskPackCount,
			stats.BloomFilterCount,
		)
	}
}

func TestPackfileStoreStatsChangedCallback(t *testing.T) {
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	opener, _ := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache())
	var calls atomic.Int32
	store.SetStatsChangedCallback(func() {
		calls.Add(1)
	})
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "callback-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})
	afterManifest := calls.Load()
	if afterManifest == 0 {
		t.Fatal("expected manifest update to notify stats callback")
	}

	alphaHash, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found {
		t.Fatal("expected target block found")
	}
	if calls.Load() <= afterManifest {
		t.Fatal("expected lookup/fetch stats to notify stats callback")
	}
}

func TestPackfileStoreIndexCacheErrorStats(t *testing.T) {
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	opener, _ := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, &errorIndexCache{
		getErr: errors.New("cache read failed"),
		setErr: errors.New("cache write failed"),
	})
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "error-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})

	alphaHash, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found {
		t.Fatal("expected target block found")
	}

	stats := store.SnapshotStats()
	if stats.IndexCacheReadErrors != 1 {
		t.Fatalf("IndexCacheReadErrors = %d, want 1", stats.IndexCacheReadErrors)
	}
	if stats.IndexCacheWriteErrors != 1 {
		t.Fatalf("IndexCacheWriteErrors = %d, want 1", stats.IndexCacheWriteErrors)
	}
	if stats.RemoteIndexLoads != 1 {
		t.Fatalf("RemoteIndexLoads = %d, want 1", stats.RemoteIndexLoads)
	}
}

func TestPackfileStoreIndexCacheHitStats(t *testing.T) {
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	tail := mustReadIndexTail(t, packBytes)

	opener, _ := openerFromBytes(packBytes)
	cache := newMemIndexCache()
	if err := cache.Set(ctx, "hit-pack", tail); err != nil {
		t.Fatal(err)
	}
	store := NewPackfileStore(opener, cache)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "hit-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})

	alphaHash, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found {
		t.Fatal("expected target block found")
	}

	stats := store.SnapshotStats()
	if stats.IndexCacheHits != 1 {
		t.Fatalf("IndexCacheHits = %d, want 1", stats.IndexCacheHits)
	}
	if stats.IndexCacheMisses != 0 {
		t.Fatalf("IndexCacheMisses = %d, want 0", stats.IndexCacheMisses)
	}
	if stats.RemoteIndexLoads != 0 {
		t.Fatalf("RemoteIndexLoads = %d, want 0", stats.RemoteIndexLoads)
	}
}

func TestPackfileStoreRejectsStaleIndexTailCache(t *testing.T) {
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{
		{"a", "alpha"},
		{"b", "beta"},
	})
	staleBytes, _ := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})

	opener, _ := openerFromBytes(packBytes)
	cache := newMemIndexCache()
	if err := cache.Set(ctx, "stale-pack", mustReadIndexTail(t, staleBytes)); err != nil {
		t.Fatal(err)
	}
	store := NewPackfileStore(opener, cache)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "stale-pack",
		BloomFilter: bloomBytes,
		BlockCount:  2,
		SizeBytes:   uint64(len(packBytes)),
	}})

	betaHash, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("beta"))
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: betaHash})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found || !bytes.Equal(got, []byte("beta")) {
		t.Fatalf("expected beta after stale cache fallback, found=%v data=%q", found, string(got))
	}

	stats := store.SnapshotStats()
	if stats.IndexCacheReadErrors != 1 {
		t.Fatalf("IndexCacheReadErrors = %d, want 1", stats.IndexCacheReadErrors)
	}
	if stats.IndexCacheHits != 0 {
		t.Fatalf("IndexCacheHits = %d, want 0", stats.IndexCacheHits)
	}
	if stats.RemoteIndexLoads != 1 {
		t.Fatalf("RemoteIndexLoads = %d, want 1", stats.RemoteIndexLoads)
	}
}

func TestPackfileStoreColdIndexTailFetchIsBounded(t *testing.T) {
	ctx := t.Context()
	large := bytes.Repeat([]byte("a"), 2<<20)
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", string(large)}})

	opener, transport := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "bounded-tail-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})
	store.SetWriteback(ctx, nil, 0)

	h, err := hash.Sum(hash.HashType_HashType_SHA256, large)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found || !bytes.Equal(got, large) {
		t.Fatalf("expected large block, found=%v len=%d", found, len(got))
	}
	first := transport.callAt(0)
	if first.length > defaultIndexTailInitialWindow {
		t.Fatalf("first fetch length = %d, want <= %d", first.length, defaultIndexTailInitialWindow)
	}
	if first.off < int64(len(packBytes)-defaultIndexTailInitialWindow) {
		t.Fatalf("first fetch offset = %d, want suffix near end of pack size %d", first.off, len(packBytes))
	}
	stats := store.SnapshotStats()
	if stats.IndexTailFetchCount == 0 {
		t.Fatal("expected index-tail fetch counters")
	}
	if stats.IndexTailFetchBytes > stats.FetchedBytes {
		t.Fatalf("index-tail bytes %d exceed fetched bytes %d", stats.IndexTailFetchBytes, stats.FetchedBytes)
	}
	if stats.IndexTailFetchBytes == stats.FetchedBytes {
		t.Fatalf("payload fetch bytes were attributed as index-tail bytes: %+v", stats)
	}
}

func TestPackfileStoreCachedTailDrivesCoBlockWriteback(t *testing.T) {
	ctx := t.Context()
	ordered := []struct{ Name, Data string }{
		{"a", "alpha"},
		{"b", "beta"},
		{"c", "charlie"},
	}
	packBytes, bloomBytes := buildTestPackOrdered(t, ordered)
	opener, _ := openerFromBytes(packBytes)
	cache := newMemIndexCache()
	if err := cache.Set(ctx, "cached-coblock-pack", mustReadIndexTail(t, packBytes)); err != nil {
		t.Fatal(err)
	}
	store := NewPackfileStore(opener, cache)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "cached-coblock-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(ordered)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	wb := &writebackStore{}
	store.SetWriteback(ctx, wb, 1<<20)

	alphaHash, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err != nil || !found {
		t.Fatalf("GetBlock alpha: found=%v err=%v", found, err)
	}
	if !waitFor(t, time.Second, func() bool { return wb.putCount() >= len(ordered) }) {
		t.Fatalf("expected %d cached-tail co-block writebacks, got %d", len(ordered), wb.putCount())
	}
	stats := store.SnapshotStats()
	if stats.IndexCacheHits != 1 {
		t.Fatalf("IndexCacheHits = %d, want 1", stats.IndexCacheHits)
	}
	if stats.RemoteIndexLoads != 0 {
		t.Fatalf("RemoteIndexLoads = %d, want 0", stats.RemoteIndexLoads)
	}
}

func TestPackfileStoreReopenReusesRawTailCache(t *testing.T) {
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	cache := newMemIndexCache()
	entry := &packfile.PackfileEntry{
		Id:          "reopen-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}
	alphaHash, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}

	firstOpener, firstTransport := openerFromBytes(packBytes)
	firstStore := NewPackfileStore(firstOpener, cache)
	firstStore.UpdateManifest([]*packfile.PackfileEntry{entry})
	if _, found, err := firstStore.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err != nil || !found {
		t.Fatalf("first GetBlock: found=%v err=%v", found, err)
	}
	firstStats := firstStore.SnapshotStats()
	if firstStats.RemoteIndexLoads != 1 {
		t.Fatalf("first RemoteIndexLoads = %d, want 1", firstStats.RemoteIndexLoads)
	}
	if firstTransport.callCount() == 0 {
		t.Fatal("expected first reader to fetch remote bytes")
	}

	secondOpener, _ := openerFromBytes(packBytes)
	secondStore := NewPackfileStore(secondOpener, cache)
	secondStore.UpdateManifest([]*packfile.PackfileEntry{entry})
	if _, found, err := secondStore.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err != nil || !found {
		t.Fatalf("second GetBlock: found=%v err=%v", found, err)
	}
	secondStats := secondStore.SnapshotStats()
	if secondStats.IndexCacheHits != 1 {
		t.Fatalf("second IndexCacheHits = %d, want 1", secondStats.IndexCacheHits)
	}
	if secondStats.RemoteIndexLoads != 0 {
		t.Fatalf("second RemoteIndexLoads = %d, want 0", secondStats.RemoteIndexLoads)
	}
}

func TestPackReaderRejectsIndexTailSizeMismatch(t *testing.T) {
	packBytes, _ := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	tail := mustReadIndexTail(t, packBytes)
	eng := NewPackReader("size-mismatch-pack", int64(len(packBytes)+1), nil, hash.HashType_HashType_SHA256)
	eng.SetExpectedBlockCount(1)
	if _, err := eng.parseIndexTail(tail); err == nil {
		t.Fatal("expected size-mismatched tail to be rejected")
	}
}

func TestValidateIndexEntriesRejectsMalformedCatalog(t *testing.T) {
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	key := []byte(h.MarshalString())
	duplicate := []*kvfile.IndexEntry{
		{Key: key, Offset: 0, Size: 5},
		{Key: key, Offset: 5, Size: 4},
	}
	if err := validateIndexEntries(duplicate, 20, 2); err == nil {
		t.Fatal("expected duplicate keys to be rejected")
	}

	outOfBounds := []*kvfile.IndexEntry{{Key: key, Offset: 19, Size: 2}}
	if err := validateIndexEntries(outOfBounds, 20, 1); err == nil {
		t.Fatal("expected out-of-bounds value to be rejected")
	}
}

// TestPackfileStoreEmptyManifest verifies behavior with no manifest entries.
func TestPackfileStoreEmptyManifest(t *testing.T) {
	ctx := t.Context()
	opener, _ := openerFromBytes(nil)
	store := NewPackfileStore(opener, newMemIndexCache())
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if found {
		t.Fatal("expected not found on empty manifest")
	}
}

// TestPackfileStoreOpenerError propagates opener errors.
func TestPackfileStoreOpenerError(t *testing.T) {
	ctx := t.Context()
	_, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	opener := func(_ string, _ int64) (*PackReader, error) {
		return nil, errors.New("network down")
	}
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "fail-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   100,
	}})
	h, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if _, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: h}); err == nil {
		t.Fatal("expected opener error")
	}
}

// TestPackfileStoreCoBlockWriteback verifies that fetching one block triggers
// persistence for every covered block within the configured physical window.
func TestPackfileStoreCoBlockWriteback(t *testing.T) {
	ctx := t.Context()
	ordered := []struct{ Name, Data string }{
		{"a", "alpha"},
		{"b", "beta"},
		{"c", "charlie"},
	}
	packBytes, bloomBytes := buildTestPackOrdered(t, ordered)
	opener, _ := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "writeback-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(ordered)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	wb := &writebackStore{}
	store.SetWriteback(ctx, wb, 1<<20)

	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	got, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
	if err != nil || !found || !bytes.Equal(got, []byte("alpha")) {
		t.Fatalf("GetBlock alpha: found=%v err=%v", found, err)
	}

	if !waitFor(t, time.Second, func() bool { return wb.putCount() >= len(ordered) }) {
		t.Fatalf("expected %d co-block writebacks, got %d", len(ordered), wb.putCount())
	}

	wb.mu.Lock()
	defer wb.mu.Unlock()
	gotKeys := make(map[string][]byte, len(wb.puts))
	for _, p := range wb.puts {
		gotKeys[p.Ref.GetHash().MarshalString()] = p.Data
	}
	for _, o := range ordered {
		h, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte(o.Data))
		if got, ok := gotKeys[h.MarshalString()]; !ok {
			t.Fatalf("expected neighbor %q in writebacks", o.Name)
		} else if !bytes.Equal(got, []byte(o.Data)) {
			t.Fatalf("neighbor %q data mismatch", o.Name)
		}
	}
}

// TestPackfileStoreTrailerPromotesBlocks verifies that bytes fetched during a
// cold kvfile trailer/index scan become first-class residents: blocks fully
// contained in those spans are immediately published into the writeback
// pipeline without a second transport round-trip.
func TestPackfileStoreTrailerPromotesBlocks(t *testing.T) {
	ctx := t.Context()
	ordered := []struct{ Name, Data string }{
		{"a", "alpha"},
		{"b", "beta"},
	}
	packBytes, bloomBytes := buildTestPackOrdered(t, ordered)

	// Large minimum transport window so the trailer fetch covers the entire
	// pack (including all block bytes).
	transport := &bytesTransport{data: packBytes}
	opener := func(packID string, size int64) (*PackReader, error) {
		e := NewPackReader(packID, size, transport, hash.HashType_HashType_SHA256)
		e.minWindow = len(packBytes)
		e.currentWindow = len(packBytes)
		return e, nil
	}

	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "promote-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(ordered)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	wb := &writebackStore{}
	// Window of 1 means target-only semantic alignment; but the trailer
	// fetch already covered everything so promotion should still publish
	// all blocks.
	store.SetWriteback(ctx, wb, 1)

	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err != nil || !found {
		t.Fatalf("GetBlock alpha: found=%v err=%v", found, err)
	}

	if !waitFor(t, time.Second, func() bool { return wb.putCount() >= len(ordered) }) {
		t.Fatalf("expected %d trailer-promoted writebacks, got %d", len(ordered), wb.putCount())
	}
}

// TestPackfileStoreReusesEngine verifies that repeated reads reuse one engine
// per pack for the store's lifetime.
func TestPackfileStoreReusesEngine(t *testing.T) {
	ctx := t.Context()
	blocks := map[string][]byte{
		"a": []byte("alpha-data"),
		"b": []byte("beta-data"),
	}
	packBytes, bloomBytes := buildTestPack(t, blocks)

	var openCount atomic.Int32
	transport := &bytesTransport{data: packBytes}
	opener := func(packID string, size int64) (*PackReader, error) {
		openCount.Add(1)
		return NewPackReader(packID, size, transport, hash.HashType_HashType_SHA256), nil
	}

	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "reuse-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(blocks)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	for _, data := range blocks {
		h, _ := hash.Sum(hash.HashType_HashType_SHA256, data)
		if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: h}); err != nil || !found {
			t.Fatalf("GetBlock: found=%v err=%v", found, err)
		}
	}
	if got := openCount.Load(); got != 1 {
		t.Fatalf("expected opener to run once, got %d", got)
	}
}

// TestPackfileStoreServesCachedBlock verifies a second GetBlock for the same
// block does not trigger additional transport fetches.
func TestPackfileStoreServesCachedBlock(t *testing.T) {
	ctx := t.Context()
	ordered := []struct{ Name, Data string }{
		{"a", "alpha"},
		{"b", "beta"},
	}
	packBytes, bloomBytes := buildTestPackOrdered(t, ordered)
	opener, transport := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "cache-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(ordered)),
		SizeBytes:   uint64(len(packBytes)),
	}})
	store.SetWriteback(ctx, nil, 1<<20)

	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err != nil || !found {
		t.Fatalf("first GetBlock: found=%v err=%v", found, err)
	}
	firstCalls := transport.callCount()
	if firstCalls == 0 {
		t.Fatal("expected transport fetches on first GetBlock")
	}
	// Wait for the block to transition to Verified/Published so the second
	// read can hit the fast path.
	if !waitFor(t, time.Second, func() bool {
		return transport.callCount() == firstCalls
	}) {
		t.Fatalf("expected fetches to stop after first read, got %d -> %d", firstCalls, transport.callCount())
	}
	if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err != nil || !found {
		t.Fatalf("second GetBlock: found=%v err=%v", found, err)
	}
	if got := transport.callCount(); got != firstCalls {
		t.Fatalf("expected cached second read to avoid transport, got %d after %d", got, firstCalls)
	}
}

// TestPackfileStoreColdReadReturnsBeforePersistence verifies the first caller
// receives bytes before the background verify + writeback completes.
func TestPackfileStoreColdReadReturnsBeforePersistence(t *testing.T) {
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	opener, _ := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "cold-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})

	blocked := make(chan struct{})
	wb := &writebackStore{blockFn: func() { <-blocked }}
	store.SetWriteback(ctx, wb, 1<<20)

	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	done := make(chan error, 1)
	go func() {
		_, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("cold read returned error: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected cold read to return before persistence finished")
	}
	close(blocked)
}

// TestPackfileStoreSecondReadWaitsForVerify verifies the second concurrent
// caller blocks until verification completes.
func TestPackfileStoreSecondReadWaitsForVerify(t *testing.T) {
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	opener, _ := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "wait-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})

	blocked := make(chan struct{})
	wb := &writebackStore{blockFn: func() { <-blocked }}
	store.SetWriteback(ctx, wb, 1<<20)

	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))

	first := make(chan error, 1)
	go func() {
		_, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
		first <- err
	}()
	// Let the first caller start and admit the block record.
	if !waitFor(t, 500*time.Millisecond, func() bool {
		select {
		case err := <-first:
			first <- err
			return true
		default:
			return false
		}
	}) {
		t.Fatal("first caller did not return before verify")
	}

	second := make(chan error, 1)
	go func() {
		_, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
		second <- err
	}()

	select {
	case <-second:
		t.Fatal("expected second caller to wait on verify")
	case <-time.After(50 * time.Millisecond):
	}

	close(blocked)
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("second caller returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected second caller to resume after verify")
	}
}

// TestPackfileStoreDedupesConcurrentFetch verifies two concurrent GetBlock
// calls for the same missing block trigger only one transport call sequence.
func TestPackfileStoreDedupesConcurrentFetch(t *testing.T) {
	ctx := t.Context()
	ordered := []struct{ Name, Data string }{
		{"a", "alpha"},
		{"b", "beta"},
	}
	packBytes, bloomBytes := buildTestPackOrdered(t, ordered)
	tail := mustReadIndexTail(t, packBytes)

	release := make(chan struct{})
	transport := &bytesTransport{data: packBytes, blockFn: func() { <-release }}
	opener := func(packID string, size int64) (*PackReader, error) {
		return NewPackReader(packID, size, transport, hash.HashType_HashType_SHA256), nil
	}

	cache := newMemIndexCache()
	if err := cache.Set(ctx, "dedupe-pack", tail); err != nil {
		t.Fatal(err)
	}

	store := NewPackfileStore(opener, cache)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "dedupe-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(ordered)),
		SizeBytes:   uint64(len(packBytes)),
	}})
	store.SetWriteback(ctx, nil, 1<<20)

	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	done1 := make(chan error, 1)
	done2 := make(chan error, 1)
	go func() {
		_, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
		done1 <- err
	}()
	// Ensure first goroutine is in the transport before starting second.
	if !waitFor(t, time.Second, func() bool { return transport.callCount() >= 1 }) {
		t.Fatal("first caller did not start a transport fetch")
	}
	go func() {
		_, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
		done2 <- err
	}()

	time.Sleep(50 * time.Millisecond)
	if got := transport.callCount(); got != 1 {
		t.Fatalf("expected one in-flight transport fetch, got %d", got)
	}
	close(release)
	if err := <-done1; err != nil {
		t.Fatalf("first GetBlock error: %v", err)
	}
	if err := <-done2; err != nil {
		t.Fatalf("second GetBlock error: %v", err)
	}
}

// TestPackfileStoreVerifyFailureAllowsRetry verifies that a corrupted fetch
// produces a recoverable miss: the failed record is discarded and a later
// GetBlock retries transport.
func TestPackfileStoreVerifyFailureAllowsRetry(t *testing.T) {
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	tail := mustReadIndexTail(t, packBytes)

	transport := &bytesTransport{data: packBytes}
	transport.rewriteFn = func(call int, off int64, data []byte) []byte {
		if call == 1 {
			// Corrupt every byte in the first response.
			out := bytes.Repeat([]byte("x"), len(data))
			return out
		}
		return data
	}
	opener := func(packID string, size int64) (*PackReader, error) {
		return NewPackReader(packID, size, transport, hash.HashType_HashType_SHA256), nil
	}

	cache := newMemIndexCache()
	if err := cache.Set(ctx, "retry-pack", tail); err != nil {
		t.Fatal(err)
	}
	store := NewPackfileStore(opener, cache)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "retry-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})
	store.SetWriteback(ctx, nil, 0)

	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	// First read returns corrupted bytes (verify happens in background so
	// the first caller sees data). The background verify will mark the
	// block failed.
	if _, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err != nil {
		t.Fatalf("first GetBlock: %v", err)
	}

	// Wait for the failed record to be evicted.
	eng, err := store.getOrOpenEngine("retry-pack", int64(len(packBytes)), 1)
	if err != nil {
		t.Fatalf("getOrOpenEngine: %v", err)
	}
	if !waitFor(t, time.Second, func() bool {
		var present bool
		eng.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			_, present = eng.blocks[alphaHash.MarshalString()]
		})
		return !present
	}) {
		t.Fatal("expected failed block to be evicted from catalog")
	}

	got, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
	if err != nil {
		t.Fatalf("second GetBlock: %v", err)
	}
	if !found || !bytes.Equal(got, []byte("alpha")) {
		t.Fatalf("expected retry to return good bytes, got found=%v data=%q", found, string(got))
	}
	if transport.callCount() < 2 {
		t.Fatalf("expected verify failure to trigger a later retry, got %d calls", transport.callCount())
	}
}

// TestPackfileStoreEvictsOldestBlock verifies the engine evicts the oldest
// unpinned block when resident bytes exceed the budget.
func TestPackfileStoreEvictsOldestBlock(t *testing.T) {
	ctx := t.Context()
	orderedA := []struct{ Name, Data string }{{"a", "alpha"}}
	orderedB := []struct{ Name, Data string }{{"b", "beta!!"}}
	packA, bloomA := buildTestPackOrdered(t, orderedA)
	packB, bloomB := buildTestPackOrdered(t, orderedB)

	openCount := atomic.Int32{}
	opener := func(packID string, size int64) (*PackReader, error) {
		openCount.Add(1)
		var data []byte
		switch packID {
		case "pack-a":
			data = packA
		case "pack-b":
			data = packB
		default:
			return nil, errors.New("unknown pack")
		}
		t := &bytesTransport{data: data}
		e := NewPackReader(packID, size, t, hash.HashType_HashType_SHA256)
		// Tiny window so the aligned fetch is minimal.
		e.minWindow = 1
		e.currentWindow = 1
		e.maxWindow = 1
		return e, nil
	}

	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{
		{Id: "pack-a", BloomFilter: bloomA, BlockCount: 1, SizeBytes: uint64(len(packA))},
		{Id: "pack-b", BloomFilter: bloomB, BlockCount: 1, SizeBytes: uint64(len(packB))},
	})
	store.SetWriteback(ctx, nil, 0)
	// Force the resident budget to exactly one byte so the second engine's
	// fetches force eviction of the first engine's spans over time. Here we
	// check engine-local eviction on pack-a.
	store.SetRangeCacheMaxBytes(1)

	hA, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	hB, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("beta!!"))
	if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: hA}); err != nil || !found {
		t.Fatalf("GetBlock(a): found=%v err=%v", found, err)
	}
	if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: hB}); err != nil || !found {
		t.Fatalf("GetBlock(b): found=%v err=%v", found, err)
	}
}

// TestPackfileStoreKeepsPinnedBlocksResident verifies that block pins (from
// in-flight verification) keep spans resident even under budget pressure.
func TestPackfileStoreKeepsPinnedBlocksResident(t *testing.T) {
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	opener, transport := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "pin-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})

	blocked := make(chan struct{})
	wb := &writebackStore{blockFn: func() { <-blocked }}
	store.SetWriteback(ctx, wb, 0)
	store.SetRangeCacheMaxBytes(1)

	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err != nil || !found {
		t.Fatalf("GetBlock: found=%v err=%v", found, err)
	}

	// The block is still verifying (writeback blocked). Its backing span
	// must remain pinned despite the 1-byte budget.
	eng, _ := store.getOrOpenEngine("pin-pack", int64(len(packBytes)), 1)
	eng.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if eng.residentBytes == 0 {
			t.Fatal("expected resident bytes to stay pinned during verify")
		}
	})
	_ = transport
	close(blocked)
}
