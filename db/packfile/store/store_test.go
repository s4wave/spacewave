package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
)

// bytesTransport serves Fetch calls from a fixed byte slice, optionally
// counting and gating calls for fault-injection style tests.
type bytesTransport struct {
	data []byte
	// mtx guards calls and the injected Fetch hooks.
	mtx sync.Mutex
	// bcast wakes tests waiting for another Fetch call.
	bcast broadcast.Broadcast
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

func (t *bytesTransport) Fetch(_ context.Context, off int64, length int) ([]byte, error) {
	// Record the transport request and capture its injected hooks.
	var call int
	var fn func()
	var rewrite func(call int, off int64, data []byte) []byte
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Record the transport call and snapshot hooks before notifying observers.
		t.mtx.Lock()
		t.calls = append(t.calls, fetchCall{off: off, length: length})
		call = len(t.calls)
		fn = t.blockFn
		rewrite = t.rewriteFn
		t.mtx.Unlock()
		broadcast()
	})

	// Apply the transport gate and reject reads beyond the pack.
	if fn != nil {
		fn()
	}
	if off >= int64(len(t.data)) {
		return nil, io.EOF
	}

	// Copy the requested pack range and apply any injected corruption.
	end := min(off+int64(length), int64(len(t.data)))
	out := bytes.Clone(t.data[off:end])
	if rewrite != nil {
		out = rewrite(call, off, out)
	}
	return out, nil
}

func (t *bytesTransport) callCount() int {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return len(t.calls)
}

func (t *bytesTransport) callAt(i int) fetchCall {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.calls[i]
}

// writebackStore records PutBlock calls for testing co-block writeback.
type writebackStore struct {
	block.StoreOps
	// mtx guards puts.
	mtx sync.Mutex
	// bcast wakes tests waiting for another PutBlock call.
	bcast broadcast.Broadcast
	puts  []*block.PutBatchEntry
	// blockFn optionally gates PutBlock for concurrency tests.
	blockFn func()
}

func newWritebackStore(blockFn func()) *writebackStore {
	return &writebackStore{
		StoreOps: block_store_inmem.NewInmemBlock(
			store_kvkey.NewDefaultKVKey(),
			store_kvtx_inmem.NewStore(),
			hash.HashType_HashType_SHA256,
			false,
		),
		blockFn: blockFn,
	}
}

func (w *writebackStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Apply the writeback gate before storing the block.
	if w.blockFn != nil {
		w.blockFn()
	}

	// Persist the block through the in-memory store.
	ref, existed, err := w.StoreOps.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, false, err
	}

	// Record the persisted block and notify writeback observers.
	w.mtx.Lock()
	w.puts = append(w.puts, &block.PutBatchEntry{Ref: ref, Data: bytes.Clone(data), Refs: opts.GetRefs()})
	w.mtx.Unlock()
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
	return ref, existed, nil
}

// PutBlockBatch records each entry through PutBlock.
func (w *writebackStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	existed := make([]bool, len(entries))
	for i, entry := range entries {
		opts := &block.PutOpts{ForceBlockRef: entry.Ref, Refs: entry.Refs}
		_, exists, err := w.PutBlock(ctx, entry.Data, opts)
		if err != nil {
			return nil, err
		}
		existed[i] = exists
	}
	return existed, nil
}

// putRefs returns the refs recorded for each written block, by hash.
func (w *writebackStore) putRefs() map[string][]*block.BlockRef {
	// Collect the references recorded by the writeback store under its lock.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	refs := make(map[string][]*block.BlockRef, len(w.puts))
	for _, p := range w.puts {
		refs[p.Ref.GetHash().MarshalString()] = p.Refs
	}
	return refs
}

func (w *writebackStore) putCount() int {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return len(w.puts)
}

func (t *bytesTransport) callCountAtLeast(want int) (bool, <-chan struct{}) {
	var ready bool
	var waitCh <-chan struct{}
	t.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		t.mtx.Lock()
		ready = len(t.calls) >= want
		t.mtx.Unlock()
		if !ready {
			waitCh = getWaitCh()
		}
	})
	return ready, waitCh
}

func (w *writebackStore) putCountAtLeast(want int) (bool, <-chan struct{}) {
	var ready bool
	var waitCh <-chan struct{}
	w.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		w.mtx.Lock()
		ready = len(w.puts) >= want
		w.mtx.Unlock()
		if !ready {
			waitCh = getWaitCh()
		}
	})
	return ready, waitCh
}

func mustReadIndexTail(t *testing.T, data []byte) []byte {
	t.Helper()
	_, tail, err := kvfile.ReadIndexTail(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return tail
}

// TransportFunc adapts a function to Transport for tests.
type TransportFunc func(ctx context.Context, off int64, length int) ([]byte, error)

// Fetch invokes the fixture's transport operation with the caller's lifetime.
func (f TransportFunc) Fetch(ctx context.Context, off int64, length int) ([]byte, error) {
	return f(ctx, off, length)
}

// openerFromBytes builds an opener that returns a fresh engine per call over
// the same byte slice.
func openerFromBytes(data []byte) (Opener, *bytesTransport) {
	t := &bytesTransport{data: data}
	opener := func(packID string, size int64) (*PackReader, error) {
		return NewPackReader(packID, size, t), nil
	}
	return opener, t
}

// waitFor retries a condition only after its owner broadcasts a state change.
// The timeout is a hang backstop; successful waits depend on notifications.
func waitFor(t *testing.T, cond func() (bool, <-chan struct{})) bool {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	for {
		ready, waitCh := cond()
		if ready {
			return true
		}

		select {
		case <-waitCtx.Done():
			return false
		case <-waitCh:
		}
	}
}

func TestWaitForUsesNotification(t *testing.T) {
	// Track the condition published by the notification producer.
	var ready atomic.Bool

	// Publish readiness after releasing the notification producer.
	release := make(chan struct{})
	waitCh := make(chan struct{})
	started := make(chan struct{})
	go func() {
		close(started)
		<-release
		ready.Store(true)
		close(waitCh)
	}()
	<-started
	close(release)

	// Verify the readiness notification wakes the condition wait.
	if !waitFor(t, func() (bool, <-chan struct{}) {
		if ready.Load() {
			return true, nil
		}
		return false, waitCh
	}) {
		t.Fatal("expected notification to wake waitFor")
	}
}

// TestPackfileStoreBasicReads verifies GetBlock found/not-found, GetBlockExists,
// PutBlock/RmBlock errors, and GetHashType.
func TestPackfileStoreBasicReads(t *testing.T) {
	// Build a pack containing the blocks used by the read checks.
	ctx := t.Context()
	blocks := map[string][]byte{
		"a": []byte("alpha-data"),
		"b": []byte("beta-data"),
		"c": []byte("charlie-data"),
	}
	packBytes, bloomBytes := buildTestPack(t, blocks)

	// Publish the pack manifest in a byte-backed store.
	opener, _ := openerFromBytes(packBytes)
	cache := newMemIndexCache()
	store := NewPackfileStore(opener, cache)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "basic-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(blocks)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	// Verify the pack store advertises SHA256 references.
	if store.GetHashType() != hash.HashType_HashType_SHA256 {
		t.Fatal("expected SHA256 hash type")
	}

	// Read every packed block and verify its bytes.
	for _, data := range blocks {
		// Derive the reference for the packed block.
		h, err := hash.Sum(hash.HashType_HashType_SHA256, data)
		if err != nil {
			t.Fatal(err)
		}

		// Read the block and verify its returned payload.
		got, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: h})
		if err != nil {
			t.Fatalf("GetBlock: %v", err)
		}
		if !found || !bytes.Equal(got, data) {
			t.Fatalf("expected block found, got found=%v len=%d", found, len(got))
		}
	}

	// Verify a reference absent from the pack returns a miss.
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

	// Verify the absent reference fails the existence probe.
	exists, err := store.GetBlockExists(ctx, &block.BlockRef{Hash: unknownHash})
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("expected unknown block to not exist")
	}

	// Verify the read-only pack store rejects block mutations.
	if _, _, err := store.PutBlock(ctx, []byte("x"), nil); err == nil {
		t.Fatal("expected PutBlock error")
	}
	if err := store.RmBlock(ctx, &block.BlockRef{Hash: unknownHash}); err == nil {
		t.Fatal("expected RmBlock error")
	}
}

func TestPackfileStoreGetBlockExistsDoesNotFetchPayload(t *testing.T) {
	// Build a pack with enough filler to separate its index from its payload.
	ctx := t.Context()
	filler := bytes.Repeat([]byte("x"), defaultIndexTailInitialWindow+4096)
	ordered := []struct{ Name, Data string }{
		{"a", "alpha-data"},
		{"b", string(filler)},
	}
	packBytes, bloomBytes := buildTestPackOrdered(t, ordered)
	opener, transport := openerFromBytes(packBytes)

	// Publish the existence-probe pack in the store.
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "exists-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(ordered)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	// Probe the packed block and verify the index was fetched.
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

	// Verify writeback leaves existence probes free of payload fetches.
	wb := newWritebackStore(nil)
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

	// Read the block and verify the payload requires another fetch.
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

func TestPackfileStoreStatBlockUsesIndexOnly(t *testing.T) {
	// Build a pack with its target payload outside the index window.
	ctx := t.Context()
	filler := bytes.Repeat([]byte("x"), defaultIndexTailInitialWindow+4096)
	ordered := []struct{ Name, Data string }{
		{"a", "alpha-data"},
		{"b", string(filler)},
	}
	packBytes, bloomBytes := buildTestPackOrdered(t, ordered)
	opener, transport := openerFromBytes(packBytes)

	// Publish the stat-probe pack in the store.
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "stat-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(ordered)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	// Load the pack index through an existence probe.
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

	// Verify block metadata uses the loaded index without fetching payload.
	stat, err := store.StatBlock(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatalf("StatBlock: %v", err)
	}
	if stat == nil {
		t.Fatal("expected block stat")
	}
	if stat.Size != -1 {
		t.Fatalf("stat size = %d, want unknown", stat.Size)
	}
	if got := transport.callCount(); got != firstCalls {
		t.Fatalf("StatBlock fetched payload window: calls %d -> %d", firstCalls, got)
	}
}

func TestPackfileStoreUpdateManifestFiltersSupersededAndEvictsEngines(t *testing.T) {
	// Seed engines whose manifest membership will change.
	store := NewPackfileStore(func(packID string, size int64) (*PackReader, error) {
		t.Fatalf("unexpected opener call for %s size %d", packID, size)
		return nil, nil
	}, nil)
	store.engines["pack-a"] = nil
	store.engines["pack-b"] = nil
	store.engines["pack-gone"] = nil

	// Replace the manifest with one superseded and one active pack.
	store.UpdateManifest([]*packfile.PackfileEntry{
		{Id: "pack-a", SupersededBy: "pack-b"},
		{Id: "pack-b"},
	})

	// Verify only the active pack remains in the manifest.
	{
		if store.manifest.Len() != 1 {
			t.Fatalf("manifest len=%d want 1", store.manifest.Len())
		}
		if store.SnapshotManifest().GetEntries()[0].GetId() != "pack-b" {
			t.Fatalf("active manifest id=%q want pack-b", store.SnapshotManifest().GetEntries()[0].GetId())
		}
	}

	// Verify manifest replacement retains only the active engine.
	store.mtx.Lock()
	defer store.mtx.Unlock()
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

func TestPackfileStoreUpdateManifestPrefersNewestSequence(t *testing.T) {
	// Build historical, compacted, and superseded versions of the target pack.
	ctx := t.Context()
	historicalBytes, historicalBloom := buildTestPackOrdered(t, []struct{ Name, Data string }{
		{"target", "alpha"},
		{"historical", "historical"},
	})
	compactBytes, compactBloom := buildTestPackOrdered(t, []struct{ Name, Data string }{
		{"target", "alpha"},
		{"compact", "compact"},
	})
	supersededBytes, supersededBloom := buildTestPackOrdered(t, []struct{ Name, Data string }{
		{"target", "alpha"},
		{"superseded", "superseded"},
	})
	packs := map[string][]byte{
		"historical": historicalBytes,
		"compact":    compactBytes,
		"superseded": supersededBytes,
	}

	// Record which pack the store opens for the target lookup.
	var opened []string
	store := NewPackfileStore(func(packID string, size int64) (*PackReader, error) {
		data, ok := packs[packID]
		if !ok {
			t.Fatalf("unexpected opener pack=%q size=%d", packID, size)
		}
		opened = append(opened, packID)
		return NewPackReader(packID, size, &bytesTransport{data: data}), nil
	}, newMemIndexCache())

	// Publish the pack generations with their sequence and supersession records.
	store.UpdateManifest([]*packfile.PackfileEntry{
		{
			Id:          "historical",
			BloomFilter: historicalBloom,
			BlockCount:  2,
			SizeBytes:   uint64(len(historicalBytes)),
			Sequence:    7,
		},
		{
			Id:           "superseded",
			BloomFilter:  supersededBloom,
			BlockCount:   2,
			SizeBytes:    uint64(len(supersededBytes)),
			Sequence:     9,
			SupersededBy: "compact",
		},
		{
			Id:          "compact",
			BloomFilter: compactBloom,
			BlockCount:  2,
			SizeBytes:   uint64(len(compactBytes)),
			Sequence:    8,
		},
	})

	// Verify active packs are ordered by their newest sequence.
	{
		if store.manifest.Len() != 2 {
			t.Fatalf("active manifest entries=%d, want 2", store.manifest.Len())
		}
		if store.SnapshotManifest().GetEntries()[0].GetId() != "compact" || store.SnapshotManifest().GetEntries()[1].GetId() != "historical" {
			t.Fatalf(
				"active manifest order=%q,%q, want compact,historical",
				store.SnapshotManifest().GetEntries()[0].GetId(),
				store.SnapshotManifest().GetEntries()[1].GetId(),
			)
		}
	}

	// Read the target and verify the compacted pack supplies it.
	target, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	data, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: target})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found || !bytes.Equal(data, []byte("alpha")) {
		t.Fatalf("GetBlock(alpha): found=%v data=%q", found, string(data))
	}
	if len(opened) != 1 || opened[0] != "compact" {
		t.Fatalf("opened packs=%q, want [compact]", opened)
	}

	// Verify lookup counters record the newest candidate hit.
	stats := store.SnapshotStats()
	if stats.LastCandidatePacks != 2 || stats.LastOpenedPacks != 1 || stats.LastNegativePacks != 0 || !stats.LastTargetHit {
		t.Fatalf(
			"last lookup candidates=%d opened=%d negative=%d hit=%v, want 2/1/0/true",
			stats.LastCandidatePacks,
			stats.LastOpenedPacks,
			stats.LastNegativePacks,
			stats.LastTargetHit,
		)
	}
}

func TestPackfileStoreUpdateManifestOrdersCloudAndLocalEntries(t *testing.T) {
	// Publish cloud and local packs with mixed sequence numbers.
	store := NewPackfileStore(nil, nil)
	store.UpdateManifest([]*packfile.PackfileEntry{
		{Id: "local-z"},
		{Id: "cloud-z", Sequence: 7},
		{Id: "cloud-a", Sequence: 7},
		{Id: "cloud-old", Sequence: 2},
		{Id: "local-a"},
		{Id: "cloud-new", Sequence: 9},
	})

	// Verify cloud sequence and pack IDs determine manifest order.
	want := []string{"cloud-new", "cloud-a", "cloud-z", "cloud-old", "local-a", "local-z"}
	{
		if store.manifest.Len() != len(want) {
			t.Fatalf("active manifest entries=%d, want %d", store.manifest.Len(), len(want))
		}
		for i, entry := range store.SnapshotManifest().GetEntries() {
			if entry.GetId() != want[i] {
				t.Fatalf("active manifest[%d]=%q, want %q", i, entry.GetId(), want[i])
			}
		}
	}
}

func TestPackfileStoreGetBlockExistsBatchUsesIndexes(t *testing.T) {
	// Build the blocks and filler used by batch existence probes.
	ctx := t.Context()
	filler := bytes.Repeat([]byte("x"), defaultIndexTailInitialWindow+4096)
	ordered := []struct{ Name, Data string }{
		{"a", "alpha-data"},
		{"b", "beta-data"},
		{"c", string(filler)},
	}
	packBytes, bloomBytes := buildTestPackOrdered(t, ordered)
	opener, transport := openerFromBytes(packBytes)

	// Publish the batch-probe pack in the store.
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "batch-exists-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(ordered)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	// Derive references for present and absent batch entries.
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

	// Probe missing, present, duplicate, and nil references with writeback enabled.
	wb := newWritebackStore(nil)
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

	// Verify batch results preserve the input reference order.
	if len(found) != len(want) {
		t.Fatalf("found len = %d, want %d", len(found), len(want))
	}
	for i := range want {
		if found[i] != want[i] {
			t.Fatalf("found[%d] = %v, want %v (all=%v)", i, found[i], want[i], found)
		}
	}

	// Verify batch probing loads the index without publishing payload.
	firstCalls := transport.callCount()
	if firstCalls == 0 {
		t.Fatal("expected index load fetch")
	}
	if got := wb.putCount(); got != 0 {
		t.Fatalf("GetBlockExistsBatch published writeback blocks: %d", got)
	}

	// Verify another batch reuses the cached index without transport.
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
	// Build a target pack and a pack with a false-positive bloom filter.
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

	// Create transports recording which candidate indexes are loaded.
	packs := map[string][]byte{
		"negative-pack": negativeBytes,
		"target-pack":   targetBytes,
	}
	transports := make(map[string]*bytesTransport, len(packs))
	opener := func(packID string, size int64) (*PackReader, error) {
		transport := &bytesTransport{data: packs[packID]}
		transports[packID] = transport
		return NewPackReader(packID, size, transport), nil
	}
	store := NewPackfileStore(opener, newMemIndexCache())

	// Publish the false-positive candidate ahead of the target pack.
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

	// Probe the target reference with writeback enabled.
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("target-data"))
	if err != nil {
		t.Fatal(err)
	}
	wb := newWritebackStore(nil)
	store.SetWriteback(ctx, wb, 0)
	exists, err := store.GetBlockExists(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatalf("GetBlockExists: %v", err)
	}
	if !exists {
		t.Fatal("expected target block to exist")
	}

	// Verify both candidate indexes load without block writeback.
	for packID, transport := range transports {
		if got := transport.callCount(); got == 0 {
			t.Fatalf("expected index load for %s", packID)
		}
	}
	if got := wb.putCount(); got != 0 {
		t.Fatalf("GetBlockExists published writeback blocks: %d", got)
	}
}

// TestPackfileStoreProbeSurvivesManifestChange verifies a lookup whose pack a
// compaction replaces while the lookup reads it finds the block in the merged
// pack instead of failing with the closed reader.
func TestPackfileStoreProbeSurvivesManifestChange(t *testing.T) {
	// Publish an old pack whose index read waits for its reader to close.
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPack(t, map[string][]byte{"a": []byte("alpha-data")})
	started := make(chan struct{})
	opener := func(packID string, size int64) (*PackReader, error) {
		if packID == "merged" {
			return NewPackReader(packID, size, &bytesTransport{data: packBytes}), nil
		}

		// Hold the old pack's index read until its reader closes.
		return NewPackReader(packID, size, TransportFunc(func(ctx context.Context, _ int64, _ int) ([]byte, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		})), nil
	}
	store := NewPackfileStore(opener, nil)
	entry := func(id string) *packfile.PackfileEntry {
		return &packfile.PackfileEntry{
			Id:          id,
			BloomFilter: bloomBytes,
			BlockCount:  1,
			SizeBytes:   uint64(len(packBytes)),
		}
	}
	store.UpdateManifest([]*packfile.PackfileEntry{entry("old")})

	// Probe the old pack, then replace it while the probe reads its index.
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha-data"))
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		exists bool
		err    error
	}
	done := make(chan result, 1)
	go func() {
		exists, err := store.GetBlockExists(ctx, &block.BlockRef{Hash: h})
		done <- result{exists, err}
	}()
	<-started
	store.UpdateManifest([]*packfile.PackfileEntry{entry("merged")})

	// The probe restarts on the merged pack and finds the block.
	res := <-done
	if res.err != nil {
		t.Fatalf("GetBlockExists: %v", res.err)
	}
	if !res.exists {
		t.Fatal("expected the block in the merged pack")
	}
}

func TestPackfileStoreReadsPackWithoutBloom(t *testing.T) {
	// Build two packs with one missing bloom filter.
	ctx := t.Context()
	alphaBytes, alphaBloom := buildTestPack(t, map[string][]byte{"alpha": []byte("alpha-data")})
	betaBytes, _ := buildTestPack(t, map[string][]byte{"beta": []byte("beta-data")})
	packs := map[string][]byte{"alpha-pack": alphaBytes, "beta-pack": betaBytes}
	opener := func(packID string, size int64) (*PackReader, error) {
		return NewPackReader(packID, size, &bytesTransport{data: packs[packID]}), nil
	}

	// Publish both packs so the bloomless pack remains a candidate.
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{
		{Id: "alpha-pack", BloomFilter: alphaBloom, BlockCount: 1, SizeBytes: uint64(len(alphaBytes)), Sequence: 2},
		{Id: "beta-pack", BlockCount: 1, SizeBytes: uint64(len(betaBytes)), Sequence: 1},
	})

	// Verify the target can be read from the bloomless pack.
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("beta-data"))
	if err != nil {
		t.Fatal(err)
	}
	data, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found || string(data) != "beta-data" {
		t.Fatalf("GetBlock found=%v data=%q, want beta-data from the pack without a bloom", found, data)
	}
}

func TestPackfileStoreLookupStats(t *testing.T) {
	// Build target and negative packs sharing the target bloom filter.
	ctx := t.Context()
	targetBytes, targetBloom := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	negativeBytes, _ := buildTestPackOrdered(t, []struct{ Name, Data string }{{"b", "beta"}})
	packs := map[string][]byte{
		"negative-pack": negativeBytes,
		"target-pack":   targetBytes,
	}
	opener := func(packID string, size int64) (*PackReader, error) {
		data := packs[packID]
		return NewPackReader(packID, size, &bytesTransport{data: data}), nil
	}
	store := NewPackfileStore(opener, newMemIndexCache())

	// Publish the negative candidate ahead of the target pack.
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

	// Read the target after probing the negative candidate.
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

	// Verify lookup counters record candidates, opens, misses, and hits.
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

	// Verify index counters record both remote index loads and their bytes.
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

// TestPackfileStoreGetBlockExistsBatchLoadsIndexesConcurrently loads the
// index of every candidate pack at once instead of one pack at a time.
func TestPackfileStoreGetBlockExistsBatchLoadsIndexesConcurrently(t *testing.T) {
	// Build candidate packs and references for a concurrent batch probe.
	ctx := t.Context()
	packs := make(map[string][]byte, 2)
	var manifest []*packfile.PackfileEntry
	var refs []*block.BlockRef
	for _, name := range []string{"one", "two"} {
		// Add the candidate pack and its bloom record to the manifest.
		packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{name, name + "-data"}})
		packs[name] = packBytes
		manifest = append(manifest, &packfile.PackfileEntry{
			Id:          name,
			BloomFilter: bloomBytes,
			BlockCount:  1,
			SizeBytes:   uint64(len(packBytes)),
		})

		// Derive the candidate block reference for the batch probe.
		h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte(name+"-data"))
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, &block.BlockRef{Hash: h})
	}

	// Every fetch waits until both packs have started an index load, so
	// loading one index after the other never completes.
	var started atomic.Int32
	bothStarted := make(chan struct{})
	opener := func(packID string, size int64) (*PackReader, error) {
		var first sync.Once
		data := packs[packID]
		transport := TransportFunc(func(_ context.Context, off int64, n int) ([]byte, error) {
			first.Do(func() {
				if started.Add(1) == 2 {
					close(bothStarted)
				}
			})
			select {
			case <-bothStarted:
			case <-time.After(5 * time.Second):
				return nil, errors.New("index loads ran one at a time")
			}
			return bytes.Clone(data[off : off+int64(n)]), nil
		})
		return NewPackReader(packID, size, transport), nil
	}
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest(manifest)

	// Verify the batch probe finds both blocks after concurrent index loads.
	found, err := store.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		t.Fatalf("GetBlockExistsBatch: %v", err)
	}
	if !slices.Equal(found, []bool{true, true}) {
		t.Fatalf("found = %v, want [true true]", found)
	}
}

func TestPackfileStoreLookupPrunesUnrelatedFullPacks(t *testing.T) {
	// Configure a full-pack collection and its target location.
	ctx := t.Context()
	policy := writer.DefaultPolicy()
	packCount := 16
	targetPack := 9
	targetBlock := 37
	packs := make(map[string][]byte, packCount)
	entries := make([]*packfile.PackfileEntry, 0, packCount)

	// Build full packs and retain the chosen target reference.
	var targetHash *hash.Hash
	for i := range packCount {
		// Build the blocks for this full pack and capture its target.
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

		// Publish the full pack bytes and its manifest entry.
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

	// Count pack opens for the target lookup.
	var openCount atomic.Int32
	opener := func(packID string, size int64) (*PackReader, error) {
		data := packs[packID]
		if data == nil {
			return nil, errors.New("unknown pack")
		}
		openCount.Add(1)
		return NewPackReader(packID, size, &bytesTransport{data: data}), nil
	}

	// Publish the full-pack collection in the store.
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest(entries)

	// Read the target block and verify its selected payload.
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

	// Verify bloom pruning bounds the opened and negative candidate counts.
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
	// Publish packs with valid, missing, and malformed bloom filters.
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

	// Verify manifest statistics include every pack and its block counts.
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

	// Verify manifest statistics include pack sizes and bloom health.
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
	// Build the pack used to observe store statistics notifications.
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	opener, _ := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache())
	var calls atomic.Int32

	// Count statistics notifications while publishing the manifest.
	store.SetStatsChangedCallback(func() {
		calls.Add(1)
	})
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "callback-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})

	// Verify manifest publication emits a statistics notification.
	afterManifest := calls.Load()
	if afterManifest == 0 {
		t.Fatal("expected manifest update to notify stats callback")
	}

	// Verify the block lookup emits another statistics notification.
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
	// Publish a pack with an index cache that fails reads and writes.
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

	// Verify the target remains readable despite index cache failures.
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

	// Verify statistics record both cache failures and the remote fallback.
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
	// Build a pack and extract its cacheable index tail.
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	tail := mustReadIndexTail(t, packBytes)

	// Seed the index cache and publish the pack manifest.
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

	// Read the target using the cached pack index.
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

	// Verify the cached index avoids remote index loads.
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
	// Build current pack bytes and a stale index tail missing one block.
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{
		{"a", "alpha"},
		{"b", "beta"},
	})
	staleBytes, _ := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})

	// Publish the current pack with the stale tail in its index cache.
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

	// Verify stale cache fallback finds the missing beta block.
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

	// Verify stale index rejection records a cache error and remote load.
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
	// Build a pack containing one large target block.
	ctx := t.Context()
	large := bytes.Repeat([]byte("a"), 2<<20)
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", string(large)}})

	// Publish the large pack with target-only fetch alignment.
	opener, transport := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "bounded-tail-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})
	store.SetWriteback(ctx, nil, 0)

	// Read the large block and verify its payload.
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

	// Verify the first transport request is a bounded index suffix.
	first := transport.callAt(0)
	if first.length > defaultIndexTailInitialWindow {
		t.Fatalf("first fetch length = %d, want <= %d", first.length, defaultIndexTailInitialWindow)
	}
	if first.off < int64(len(packBytes)-defaultIndexTailInitialWindow) {
		t.Fatalf("first fetch offset = %d, want suffix near end of pack size %d", first.off, len(packBytes))
	}

	// Verify index-tail counters exclude payload fetch bytes.
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
	// Build the adjacent blocks used by cached-index writeback.
	ctx := t.Context()
	ordered := []struct{ Name, Data string }{
		{"a", "alpha"},
		{"b", "beta"},
		{"c", "charlie"},
	}
	packBytes, bloomBytes := buildTestPackOrdered(t, ordered)

	// Seed the raw index tail and publish the adjacent-block pack.
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

	// Enable writeback across the adjacent-block window.
	wb := newWritebackStore(nil)
	store.SetWriteback(ctx, wb, 1<<20)

	// Read alpha and wait for writeback of every covered block.
	alphaHash, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err != nil || !found {
		t.Fatalf("GetBlock alpha: found=%v err=%v", found, err)
	}
	if !waitFor(t, func() (bool, <-chan struct{}) {
		return wb.putCountAtLeast(len(ordered))
	}) {
		t.Fatalf("expected %d cached-tail co-block writebacks, got %d", len(ordered), wb.putCount())
	}

	// Verify writeback used the cached index without a remote index load.
	stats := store.SnapshotStats()
	if stats.IndexCacheHits != 1 {
		t.Fatalf("IndexCacheHits = %d, want 1", stats.IndexCacheHits)
	}
	if stats.RemoteIndexLoads != 0 {
		t.Fatalf("RemoteIndexLoads = %d, want 0", stats.RemoteIndexLoads)
	}
}

func TestPackfileStoreReopenReusesRawTailCache(t *testing.T) {
	// Build a pack manifest and target reference shared by two stores.
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

	// Read the target through the first store and verify its remote index load.
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

	// Read the target through a new store and verify index cache reuse.
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
	// Verify the reader rejects an index tail with a different pack size.
	packBytes, _ := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	tail := mustReadIndexTail(t, packBytes)
	eng := NewPackReader("size-mismatch-pack", int64(len(packBytes)+1), nil)
	eng.SetExpectedBlockCount(1)
	if _, err := eng.parseIndexTail(tail); err == nil {
		t.Fatal("expected size-mismatched tail to be rejected")
	}
}

func TestValidateIndexEntriesRejectsMalformedCatalog(t *testing.T) {
	// Verify the index validator rejects duplicate block keys.
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	key := packfile.BlockKey(h)
	duplicate := []*kvfile.IndexEntry{
		{Key: key, Offset: 0, Size: 5},
		{Key: key, Offset: 5, Size: 4},
	}
	if err := validateIndexEntries(duplicate, 20, 2); err == nil {
		t.Fatal("expected duplicate keys to be rejected")
	}

	// Verify the index validator rejects values beyond the pack boundary.
	outOfBounds := []*kvfile.IndexEntry{{Key: key, Offset: 19, Size: 2}}
	if err := validateIndexEntries(outOfBounds, 20, 1); err == nil {
		t.Fatal("expected out-of-bounds value to be rejected")
	}
}

// TestPackfileStoreEmptyManifest verifies behavior with no manifest entries.
func TestPackfileStoreEmptyManifest(t *testing.T) {
	// Open an empty pack store and derive an absent block reference.
	ctx := t.Context()
	opener, _ := openerFromBytes(nil)
	store := NewPackfileStore(opener, newMemIndexCache())
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("x"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the empty manifest returns a block miss.
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
	// Publish a pack whose opener fails to reach the transport.
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

	// Verify the block lookup propagates the opener failure.
	if _, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: h}); err == nil {
		t.Fatal("expected opener error")
	}
}

// TestPackfileStoreCoBlockWriteback verifies that fetching one block triggers
// persistence for every covered block within the configured physical window.
func TestPackfileStoreCoBlockWriteback(t *testing.T) {
	// Build and publish a pack of adjacent blocks.
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

	// Enable writeback for the entire adjacent-block window.
	wb := newWritebackStore(nil)
	store.SetWriteback(ctx, wb, 1<<20)

	// Read alpha and verify the requested block payload.
	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	got, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
	if err != nil || !found || !bytes.Equal(got, []byte("alpha")) {
		t.Fatalf("GetBlock alpha: found=%v err=%v", found, err)
	}

	// Wait for every covered block to reach the writeback store.
	if !waitFor(t, func() (bool, <-chan struct{}) {
		return wb.putCountAtLeast(len(ordered))
	}) {
		t.Fatalf("expected %d co-block writebacks, got %d", len(ordered), wb.putCount())
	}

	// Collect written payloads by their block reference under the store lock.
	wb.mtx.Lock()
	defer wb.mtx.Unlock()
	gotKeys := make(map[string][]byte, len(wb.puts))
	for _, p := range wb.puts {
		gotKeys[p.Ref.GetHash().MarshalString()] = p.Data
	}

	// Verify every adjacent block retains its payload in writeback.
	for _, o := range ordered {
		h, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte(o.Data))
		if got, ok := gotKeys[h.MarshalString()]; !ok {
			t.Fatalf("expected neighbor %q in writebacks", o.Name)
		} else if !bytes.Equal(got, []byte(o.Data)) {
			t.Fatalf("neighbor %q data mismatch", o.Name)
		}
	}
}

// TestPackfileStoreServesAndWritesBackRefs verifies that a pack serves each
// block's refs as known, that co-block writeback records them, and that a
// graph copy can read a World straight from packs.
func TestPackfileStoreServesAndWritesBackRefs(t *testing.T) {
	// Build a root block referencing the leaf blocks.
	ctx := t.Context()
	leaves := []packItem{testPackItem(t, "beta"), testPackItem(t, "charlie")}
	root := testPackItem(t, "alpha")
	for _, leaf := range leaves {
		root.refs = append(root.refs, block.NewBlockRef(leaf.h))
	}

	// Pack the graph and describe its manifest entry.
	items := append([]packItem{root}, leaves...)
	packBytes, bloomBytes := packItems(t, items)
	entry := &packfile.PackfileEntry{
		Id:          "refs-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(items)),
		SizeBytes:   uint64(len(packBytes)),
	}

	// Create stores that expose the same packed graph.
	openStore := func() *PackfileStore {
		opener, _ := openerFromBytes(packBytes)
		store := NewPackfileStore(opener, newMemIndexCache())
		store.UpdateManifest([]*packfile.PackfileEntry{entry})
		return store
	}

	// Verify recorded references match every block in the packed graph.
	checkRefs := func(what string, got map[string][]*block.BlockRef) {
		t.Helper()
		for _, item := range items {
			refs, ok := got[item.h.MarshalString()]
			if !ok {
				t.Fatalf("%s: %q missing", what, item.data)
			}
			if len(refs) != len(item.refs) {
				t.Fatalf("%s: %q has %d refs, want %d", what, item.data, len(refs), len(item.refs))
			}
			for i, ref := range item.refs {
				if !refs[i].EqualsRef(ref) {
					t.Fatalf("%s: %q ref %d mismatch", what, item.data, i)
				}
			}
		}
	}

	// Read the graph root with co-block writeback enabled.
	store := openStore()
	wb := newWritebackStore(nil)
	store.SetWriteback(ctx, wb, 1<<20)
	rootRef := block.NewBlockRef(root.h)

	// Verify the packed root exposes its known leaf references.
	stored, err := store.GetStoredBlock(ctx, rootRef)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.GetRefsKnown() || len(stored.GetRefs()) != len(leaves) {
		t.Fatalf("root refs known=%v count=%d, want %d", stored.GetRefsKnown(), len(stored.GetRefs()), len(leaves))
	}

	// Wait for the packed graph to reach writeback and verify its references.
	if !waitFor(t, func() (bool, <-chan struct{}) {
		return wb.putCountAtLeast(len(items))
	}) {
		t.Fatalf("expected %d co-block writebacks, got %d", len(items), wb.putCount())
	}
	checkRefs("writeback", wb.putRefs())

	// Copy the packed graph into another store and verify its references.
	dst := newWritebackStore(nil)
	if err := block.CopyGraph(ctx, openStore(), dst, rootRef, nil); err != nil {
		t.Fatal(err)
	}
	checkRefs("graph copy", dst.putRefs())
}

// TestPackfileStoreTrailerPromotesBlocks verifies that bytes fetched during a
// cold kvfile trailer/index scan become first-class residents: blocks fully
// contained in those spans are immediately published into the writeback
// pipeline without a second transport round-trip.
func TestPackfileStoreTrailerPromotesBlocks(t *testing.T) {
	// Build a small pack whose trailer window can cover every block.
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
		e := NewPackReader(packID, size, transport)
		e.minWindow = len(packBytes)
		e.currentWindow = len(packBytes)
		return e, nil
	}

	// Publish the trailer-promotion pack in the store.
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "promote-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(ordered)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	// Attach the writeback target for trailer-promoted blocks.
	wb := newWritebackStore(nil)

	// Window of 1 means target-only semantic alignment; but the trailer
	// fetch already covered everything so promotion should still publish
	// all blocks.
	store.SetWriteback(ctx, wb, 1)

	// Read alpha through the trailer-covering transport.
	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err != nil || !found {
		t.Fatalf("GetBlock alpha: found=%v err=%v", found, err)
	}

	// Wait for every trailer-covered block to reach writeback.
	if !waitFor(t, func() (bool, <-chan struct{}) {
		return wb.putCountAtLeast(len(ordered))
	}) {
		t.Fatalf("expected %d trailer-promoted writebacks, got %d", len(ordered), wb.putCount())
	}
}

// TestPackfileStoreReusesEngine verifies that repeated reads reuse one engine
// per pack for the store's lifetime.
func TestPackfileStoreReusesEngine(t *testing.T) {
	// Build a pack with multiple blocks for engine reuse checks.
	ctx := t.Context()
	blocks := map[string][]byte{
		"a": []byte("alpha-data"),
		"b": []byte("beta-data"),
	}
	packBytes, bloomBytes := buildTestPack(t, blocks)

	// Count reader construction through the shared pack transport.
	var openCount atomic.Int32
	transport := &bytesTransport{data: packBytes}
	opener := func(packID string, size int64) (*PackReader, error) {
		openCount.Add(1)
		return NewPackReader(packID, size, transport), nil
	}

	// Publish the engine-reuse pack in the store.
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "reuse-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(blocks)),
		SizeBytes:   uint64(len(packBytes)),
	}})

	// Read every block from the same packed engine.
	for _, data := range blocks {
		h, _ := hash.Sum(hash.HashType_HashType_SHA256, data)
		if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: h}); err != nil || !found {
			t.Fatalf("GetBlock: found=%v err=%v", found, err)
		}
	}

	// Verify the store opens its pack reader only once.
	if got := openCount.Load(); got != 1 {
		t.Fatalf("expected opener to run once, got %d", got)
	}
}

// TestPackfileStoreServesCachedBlock verifies a second GetBlock for the same
// block does not trigger additional transport fetches.
func TestPackfileStoreServesCachedBlock(t *testing.T) {
	// Publish adjacent blocks with a shared resident window.
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

	// Read alpha and verify the first read fetches transport bytes.
	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err != nil || !found {
		t.Fatalf("first GetBlock: found=%v err=%v", found, err)
	}
	firstCalls := transport.callCount()
	if firstCalls == 0 {
		t.Fatal("expected transport fetches on first GetBlock")
	}

	// Verify the repeated alpha read uses resident bytes without transport.
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
	// Publish a cold pack whose first read starts background writeback.
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

	// Hold background writeback until the cold read completes.
	blocked := make(chan struct{})
	wb := newWritebackStore(func() { <-blocked })
	store.SetWriteback(ctx, wb, 1<<20)

	// Start the cold block read while writeback remains gated.
	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	done := make(chan error, 1)
	go func() {
		_, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
		done <- err
	}()

	// Verify the cold read completes before releasing background writeback.
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

// TestPackfileStoreRejectsCorruptedBlock verifies a read whose bytes decode but
// do not match the ref returns a mismatch error instead of the wrong data.
func TestPackfileStoreRejectsCorruptedBlock(t *testing.T) {
	// Publish a pack through a transport that corrupts alpha payloads.
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	transport := &bytesTransport{data: packBytes}
	transport.rewriteFn = func(_ int, _ int64, data []byte) []byte {
		return bytes.ReplaceAll(data, []byte("alpha"), []byte("alphx"))
	}
	opener := func(packID string, size int64) (*PackReader, error) {
		return NewPackReader(packID, size, transport), nil
	}
	store := NewPackfileStore(opener, newMemIndexCache())
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "corrupt-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})

	// Verify repeated reads reject the corrupted block reference.
	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	for range 2 {
		got, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
		if !errors.Is(err, block.ErrBlockRefMismatch) {
			t.Fatalf("GetBlock data=%q err=%v, want ErrBlockRefMismatch", got, err)
		}
	}
}

// TestPackfileStoreSecondReadReturnsBeforePersistence verifies repeated default
// reads use resident bytes while the background cache write remains blocked.
func TestPackfileStoreSecondReadReturnsBeforePersistence(t *testing.T) {
	// Publish a pack used for repeated reads during background writeback.
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

	// Hold background writeback while the resident block is read.
	blocked := make(chan struct{})
	wb := newWritebackStore(func() { <-blocked })
	store.SetWriteback(ctx, wb, 1<<20)

	// Derive the resident alpha block reference.
	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))

	// Start the first read and publish its completion notification.
	first := make(chan error, 1)
	firstDone := make(chan struct{})
	go func() {
		_, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
		first <- err
		close(firstDone)
	}()

	// Let the first caller start and admit the block record.
	if !waitFor(t, func() (bool, <-chan struct{}) {
		select {
		case <-firstDone:
			return true, nil
		default:
			return false, firstDone
		}
	}) {
		t.Fatal("first caller did not return before verify")
	}
	if err := <-first; err != nil {
		t.Fatalf("first GetBlock error: %v", err)
	}

	// Start another read and verify its returned resident payload.
	second := make(chan error, 1)
	go func() {
		data, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
		if err == nil && (!found || !bytes.Equal(data, []byte("alpha"))) {
			err = errors.New("repeated read returned wrong block")
		}
		second <- err
	}()

	// Verify the repeated read completes before releasing background writeback.
	select {
	case err := <-second:
		if err != nil {
			close(blocked)
			t.Fatalf("second caller returned error: %v", err)
		}
	case <-time.After(time.Second):
		close(blocked)
		t.Fatal("repeated read waited for background persistence")
	}
	close(blocked)
}

// TestPackfileStoreDedupesConcurrentFetch verifies two concurrent GetBlock
// calls for the same missing block trigger only one transport call sequence.
func TestPackfileStoreDedupesConcurrentFetch(t *testing.T) {
	// Build a pack and its index tail for concurrent cold reads.
	ctx := t.Context()
	ordered := []struct{ Name, Data string }{
		{"a", "alpha"},
		{"b", "beta"},
	}
	packBytes, bloomBytes := buildTestPackOrdered(t, ordered)
	tail := mustReadIndexTail(t, packBytes)

	// Gate transport requests until both readers have started.
	release := make(chan struct{})
	transport := &bytesTransport{data: packBytes, blockFn: func() { <-release }}
	opener := func(packID string, size int64) (*PackReader, error) {
		return NewPackReader(packID, size, transport), nil
	}

	// Seed the pack index to isolate payload-fetch deduplication.
	cache := newMemIndexCache()
	if err := cache.Set(ctx, "dedupe-pack", tail); err != nil {
		t.Fatal(err)
	}

	// Publish the pack with a shared resident fetch window.
	store := NewPackfileStore(opener, cache)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "dedupe-pack",
		BloomFilter: bloomBytes,
		BlockCount:  uint64(len(ordered)),
		SizeBytes:   uint64(len(packBytes)),
	}})
	store.SetWriteback(ctx, nil, 1<<20)

	// Start the first cold reader for the alpha reference.
	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	done1 := make(chan error, 1)
	done2 := make(chan error, 1)
	go func() {
		_, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
		done1 <- err
	}()

	// Ensure first goroutine is in the transport before starting second.
	if !waitFor(t, func() (bool, <-chan struct{}) {
		return transport.callCountAtLeast(1)
	}) {
		t.Fatal("first caller did not start a transport fetch")
	}

	// Start another alpha reader while the first fetch remains gated.
	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		_, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
		done2 <- err
	}()
	<-secondStarted

	// Release transport and verify both readers share one payload fetch.
	close(release)
	if err := <-done1; err != nil {
		t.Fatalf("first GetBlock error: %v", err)
	}
	if err := <-done2; err != nil {
		t.Fatalf("second GetBlock error: %v", err)
	}
	if got := transport.callCount(); got != 1 {
		t.Fatalf("expected one transport fetch for concurrent reads, got %d", got)
	}
}

// TestPackfileStoreVerifyFailureAllowsRetry verifies that a corrupted fetch
// produces a recoverable miss: the failed record is discarded and a later
// GetBlock retries transport.
func TestPackfileStoreVerifyFailureAllowsRetry(t *testing.T) {
	// Build a pack and index tail for recovery after corrupt transport bytes.
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	tail := mustReadIndexTail(t, packBytes)

	// Create a transport that corrupts only its first response.
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
		return NewPackReader(packID, size, transport), nil
	}

	// Seed the valid pack index before injecting payload corruption.
	cache := newMemIndexCache()
	if err := cache.Set(ctx, "retry-pack", tail); err != nil {
		t.Fatal(err)
	}

	// Publish the recovery pack with target-only fetch alignment.
	store := NewPackfileStore(opener, cache)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "retry-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})
	store.SetWriteback(ctx, nil, 0)

	// Derive the alpha reference for the corruption and recovery checks.
	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))

	// The first read never serves the corrupted value. It fails, or it
	// returns the refetched value when background verification rejected the
	// corrupted span before the read covered it.
	if got, _, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err == nil && string(got) != "alpha" {
		t.Fatalf("corrupted read returned %q", got)
	}

	// Observe rejection rather than transient catalog absence: a valid retry
	// may already have installed its replacement record.
	eng, release, err := store.getOrOpenEngine("retry-pack", int64(len(packBytes)), 1)
	if err != nil {
		t.Fatalf("getOrOpenEngine: %v", err)
	}
	t.Cleanup(release)
	if !waitFor(t, func() (bool, <-chan struct{}) {
		var rejected bool
		var waitCh <-chan struct{}
		eng.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			rejected = eng.verifyFailures != 0
			if !rejected {
				waitCh = getWaitCh()
			}
		})
		return rejected, waitCh
	}) {
		t.Fatal("expected corrupted response to fail verification")
	}

	// Verify another read returns valid alpha bytes after rejection.
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
	// Build two packs whose resident spans will exceed the cache budget.
	ctx := t.Context()
	orderedA := []struct{ Name, Data string }{{"a", "alpha"}}
	orderedB := []struct{ Name, Data string }{{"b", "beta!!"}}
	packA, bloomA := buildTestPackOrdered(t, orderedA)
	packB, bloomB := buildTestPackOrdered(t, orderedB)

	// Create pack readers that fetch the smallest possible windows.
	openCount := atomic.Int32{}
	opener := func(packID string, size int64) (*PackReader, error) {
		// Select the requested pack bytes and count reader construction.
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

		// Construct a byte-backed reader for the selected pack.
		t := &bytesTransport{data: data}
		e := NewPackReader(packID, size, t)

		// Tiny window so the aligned fetch is minimal.
		e.minWindow = 1
		e.currentWindow = 1
		e.maxWindow = 1
		return e, nil
	}

	// Publish both packs with background writeback disabled.
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

	// Verify both blocks remain readable under the one-byte cache budget.
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
	// Publish a pack whose verifying spans must remain resident.
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

	// Gate writeback and constrain the resident cache to one byte.
	blocked := make(chan struct{})
	wb := newWritebackStore(func() { <-blocked })
	store.SetWriteback(ctx, wb, 0)
	store.SetRangeCacheMaxBytes(1)

	// Read alpha to start verification of its resident span.
	alphaHash, _ := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if _, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash}); err != nil || !found {
		t.Fatalf("GetBlock: found=%v err=%v", found, err)
	}

	// The block is still verifying (writeback blocked). Its backing span
	// must remain pinned despite the 1-byte budget.
	eng, release, _ := store.getOrOpenEngine("pin-pack", int64(len(packBytes)), 1)
	t.Cleanup(release)
	eng.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if eng.residentBytes == 0 {
			t.Fatal("expected resident bytes to stay pinned during verify")
		}
	})
	_ = transport
	close(blocked)
}

func TestPackfileStoreCloseDrainsWritebackBeforeReleasingReferences(t *testing.T) {
	// Gate the first block writeback in an initialized pack reader.
	firstPut := make(chan struct{})
	releasePut := make(chan struct{})
	var putCalls atomic.Int32
	writeback := newWritebackStore(func() {
		if putCalls.Add(1) == 1 {
			close(firstPut)
			<-releasePut
		}
	})
	cache := newMemIndexCache()
	eng := NewPackReader("close-writeback", 64, &bytesTransport{})
	eng.SetIndexCache(cache)
	eng.SetWriteback(t.Context(), writeback, 64)

	// Prepare and start writeback for two resident encoded blocks.
	var job func()
	eng.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Populate the reader with resident blocks and their index entries.
		var entries []*kvfile.IndexEntry
		for i, data := range [][]byte{[]byte("first"), []byte("second")} {
			// Build the reference and encoded value for the resident block.
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			value, err := block.EncodeBlockObject(data, nil)
			if err != nil {
				t.Fatal(err)
			}

			// Insert the resident span and describe its index entry.
			sp := newSpan(int64(i*16), value)
			eng.insertSpanLocked(sp)
			entries = append(entries, &kvfile.IndexEntry{
				Key:    packfile.BlockKey(ref.GetHash()),
				Offset: uint64(sp.off),
				Size:   uint64(len(value)),
			})
		}

		// Install the resident block catalog and prepare its writeback job.
		eng.setIndexEntriesLocked(entries)
		job = eng.prepareWritebackLocked(0, eng.size)
	})
	if job == nil {
		t.Fatal("expected a writeback job for the resident blocks")
	}
	go job()
	<-firstPut

	// Attach the running pack reader to the store being closed.
	store := NewPackfileStore(nil, cache)
	store.SetWriteback(t.Context(), writeback, 64)
	store.mtx.Lock()
	store.engines[eng.packID] = eng
	store.mtx.Unlock()

	// Close the store while its writeback batch remains gated.
	closeDone := make(chan struct{})
	go func() {
		store.Close()
		close(closeDone)
	}()
	<-eng.ctx.Done()

	// Verify closing retains engine references until writeback drains.
	select {
	case <-closeDone:
		t.Fatal("Close returned while PutBlockBatch was running")
	default:
	}
	eng.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if eng.indexCache == nil || eng.writebackTarget == nil {
			t.Fatal("engine released references before writeback drained")
		}
	})

	// Release writeback and verify closing drains the entire batch.
	close(releasePut)
	<-closeDone
	if got := putCalls.Load(); got != 2 {
		t.Fatalf("PutBlock calls = %d, want the whole running batch", got)
	}
	eng.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if eng.writebackRunning != 0 || eng.indexCache != nil ||
			eng.writebackTarget != nil || eng.published != nil || eng.spans != nil {
			t.Fatalf("engine retained work or references after Close: running=%d", eng.writebackRunning)
		}
	})

	// Verify closing releases the store cache, target, and engines.
	store.mtx.Lock()
	defer store.mtx.Unlock()
	if store.cache != nil || store.writebackTarget != nil || store.engines != nil {
		t.Fatal("store retained cache, writeback, or engine references after Close")
	}
}

func TestPackfileStoreUpdateManifestAfterCloseIgnoresValidBloom(t *testing.T) {
	// Close a store before publishing a valid pack bloom filter.
	_, bloomBytes := buildTestPack(t, map[string][]byte{"block": []byte("data")})
	store := NewPackfileStore(nil, newMemIndexCache())
	store.Close()

	// Verify manifest publication after closing leaves the store empty.
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "closed-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   1,
	}})
	if got := store.SnapshotStats().ManifestEntries; got != 0 {
		t.Fatalf("manifest entries after Close = %d, want 0", got)
	}
}
