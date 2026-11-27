package store

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// indexPackDirEnv selects a directory of real packfiles to compare in
// TestTailIndexMatchesParsedIndexOnPackDir.
const indexPackDirEnv = "PACKFILE_INDEX_PACK_DIR"

// buildBothIndexes builds the parsed and in-place index of a pack tail with
// the reader the store uses, returning each build's error.
func buildBothIndexes(tail []byte, size int64, blockCount uint64) (*parsedIndex, *tailIndex, error, error) {
	e := NewPackReader("test-pack", size, nil)
	defer e.Close()
	e.SetExpectedBlockCount(blockCount)
	entries, perr := e.parseIndexTail(tail)
	var p *parsedIndex
	if perr == nil {
		p = newParsedIndex(entries)
	}
	t, terr := newTailIndex(bytes.Clone(tail), uint64(size), blockCount)
	return p, t, perr, terr
}

// absentKeys derives keys near each key of an index: one byte changed at each
// end, truncated, extended, and the empty and oversize keys.
func absentKeys(keys [][]byte) [][]byte {
	out := [][]byte{nil, {}, {0}, bytes.Repeat([]byte{0xff}, 64)}
	for _, k := range keys {
		last := bytes.Clone(k)
		last[len(last)-1] ^= 1
		first := bytes.Clone(k)
		first[0] ^= 1
		out = append(out,
			last, first,
			bytes.Clone(k[:len(k)-1]),
			append(bytes.Clone(k), 0),
			append(bytes.Clone(k), k...),
			bytes.Clone(k[:len(k)/2]),
		)
	}
	return out
}

// requireSameIndex fails the test unless the in-place index answers every
// probe exactly as the parsed index does: every key, near-miss keys, the
// offset order, and a seek at and around every extent boundary.
func requireSameIndex(t *testing.T, p *parsedIndex, ti *tailIndex, size int64) {
	t.Helper()
	if p.len() != ti.len() {
		t.Fatalf("len: parsed %d, in place %d", p.len(), ti.len())
	}

	// Every present key finds the same extent, and near-miss keys agree.
	var keys [][]byte
	for _, e := range p.byKey {
		keys = append(keys, e.GetKey())
	}
	probes := slices.Clone(keys)
	probes = append(probes, absentKeys(keys)...)
	for _, k := range probes {
		pe, pok := p.find(k)
		te, tok := ti.find(k)
		if pe != te || pok != tok {
			t.Fatalf("find(%x): parsed (%v, %v), in place (%v, %v)", k, pe, pok, te, tok)
		}
	}
	for _, k := range keys {
		if _, ok := ti.find(k); !ok {
			t.Fatalf("find(%x): present key not found in place", k)
		}
	}

	// The offset order lists the same keys and extents.
	seeks := []int64{-1, 0, 1, size, size + 1}
	for pos := range p.len() {
		pk, pe := p.atOffset(pos)
		tk, te := ti.atOffset(pos)
		if !bytes.Equal(pk, tk) || pe != te {
			t.Fatalf("atOffset(%d): parsed (%x, %v), in place (%x, %v)", pos, pk, pe, tk, te)
		}
		seeks = append(seeks, pe.off-1, pe.off, pe.off+1, pe.end-1, pe.end, pe.end+1)
	}
	for _, s := range seeks {
		if a, b := p.seekEnd(s), ti.seekEnd(s); a != b {
			t.Fatalf("seekEnd(%d): parsed %d, in place %d", s, a, b)
		}
		if a, b := p.seekOffset(s), ti.seekOffset(s); a != b {
			t.Fatalf("seekOffset(%d): parsed %d, in place %d", s, a, b)
		}
	}
}

// requireSameVerdict builds both indexes from tail, requires both builds to
// fail or succeed, and compares the indexes when both builds succeed.
func requireSameVerdict(t *testing.T, tail []byte, size int64, blockCount uint64, what string) bool {
	t.Helper()
	p, ti, perr, terr := buildBothIndexes(tail, size, blockCount)
	if (perr == nil) != (terr == nil) {
		t.Fatalf("%s: parsed err %v, in place err %v", what, perr, terr)
	}
	if perr != nil {
		return false
	}
	requireSameIndex(t, p, ti, size)
	return true
}

// craftedPack builds a pack of payload bytes followed by an index of entries
// written in the order given, and returns the pack's bytes. kvfile.WriteIndex
// would sort the entries and reject duplicates. Entries may share offsets.
func craftedPack(t *testing.T, payload int, entries []*kvfile.IndexEntry) []byte {
	t.Helper()
	buf := bytes.NewBuffer(make([]byte, payload))
	var poss []uint64
	for _, e := range entries {
		raw, err := e.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(raw)
		poss = append(poss, uint64(buf.Len()))
		buf.Write(protobuf_go_lite.AppendVarint(nil, uint64(len(raw))))
	}
	poss = append(poss, uint64(len(entries)))
	for _, pos := range poss {
		buf.Write(binary.LittleEndian.AppendUint64(nil, pos))
	}
	return buf.Bytes()
}

// sortedEntries sorts entries by key, as a valid index lists them.
func sortedEntries(entries []*kvfile.IndexEntry) []*kvfile.IndexEntry {
	slices.SortStableFunc(entries, func(a, b *kvfile.IndexEntry) int { return bytes.Compare(a.Key, b.Key) })
	return entries
}

// testBlockHash returns the hash key of block i.
func testBlockHash(t *testing.T, hashType hash.HashType, i int) []byte {
	t.Helper()
	h, err := hash.Sum(hashType, fmt.Appendf(nil, "block %d", i))
	if err != nil {
		t.Fatal(err)
	}
	return packfile.BlockKey(h)
}

// TestTailIndexMatchesParsedIndex compares the in-place index with the parsed
// index on built and crafted packs, and on corrupt and truncated tails, which
// must fail in both.
func TestTailIndexMatchesParsedIndex(t *testing.T) {
	// Real packs of many sizes, with blocks of mixed lengths so the payload
	// order differs from the key order.
	for _, n := range []int{1, 2, 3, 17, 300, 4096} {
		var items []packItem
		for i := range n {
			items = append(items, testPackItem(t, fmt.Sprintf("block %d %s", i, bytes.Repeat([]byte{'x'}, (i*7919)%509))))
		}
		data, _ := packItems(t, items)
		tail := mustReadIndexTail(t, data)
		if !requireSameVerdict(t, tail, int64(len(data)), uint64(n), fmt.Sprintf("pack of %d", n)) {
			t.Fatalf("pack of %d: tail rejected", n)
		}
	}

	// Crafted packs: offsets that tie, empty values, a single entry, and every
	// supported hash type.
	for _, hashType := range []hash.HashType{hash.HashType_HashType_SHA256, hash.HashType_HashType_BLAKE3} {
		var entries []*kvfile.IndexEntry
		offs := []uint64{40, 10, 10, 10, 0, 90, 40, 0}
		sizes := []uint64{10, 0, 0, 5, 10, 10, 0, 0}
		for i := range offs {
			entries = append(entries, &kvfile.IndexEntry{Key: testBlockHash(t, hashType, i), Offset: offs[i], Size: sizes[i]})
		}
		data := craftedPack(t, 100, sortedEntries(entries))
		tail := mustReadIndexTail(t, data)
		if !requireSameVerdict(t, tail, int64(len(data)), uint64(len(entries)), "crafted ties") {
			t.Fatal("crafted ties: tail rejected")
		}
	}

	// A tail corrupted in any byte, or cut at any point, or checked against a
	// different size or block count, fails in both or builds the same index.
	var items []packItem
	for i := range 6 {
		items = append(items, testPackItem(t, fmt.Sprintf("corrupt %d", i)))
	}
	data, _ := packItems(t, items)
	size := int64(len(data))
	tail := mustReadIndexTail(t, data)
	var built, failed int
	count := func(ok bool) {
		if ok {
			built++
		} else {
			failed++
		}
	}
	for i := range tail {
		for _, mut := range []func(byte) byte{func(b byte) byte { return b ^ 0xff }, func(b byte) byte { return b ^ 1 }, func(byte) byte { return 0 }} {
			c := bytes.Clone(tail)
			c[i] = mut(c[i])
			count(requireSameVerdict(t, c, size, 6, fmt.Sprintf("byte %d", i)))
		}
	}
	for i := range len(tail) + 1 {
		count(requireSameVerdict(t, tail[:i], size, 6, fmt.Sprintf("cut to %d", i)))
		count(requireSameVerdict(t, tail[i:], size, 6, fmt.Sprintf("cut from %d", i)))
	}
	for _, d := range []int64{-9, -1, 0, 1, 9} {
		for _, bc := range []uint64{0, 5, 6, 7} {
			count(requireSameVerdict(t, tail, size+d, bc, fmt.Sprintf("size %+d count %d", d, bc)))
		}
	}
	if built == 0 || failed == 0 {
		t.Fatalf("corruption sweep built %d and failed %d tails, want both", built, failed)
	}
}

// TestTailIndexRejectsMalformedEntries checks the entry rules the parsed index
// enforces: unsorted or duplicate keys, an empty or invalid key, and an
// extent beyond the tail all fail in both, and an empty pack builds in both.
func TestTailIndexRejectsMalformedEntries(t *testing.T) {
	k0, k1 := testBlockHash(t, hash.HashType_HashType_SHA256, 0), testBlockHash(t, hash.HashType_HashType_SHA256, 1)
	if bytes.Compare(k0, k1) > 0 {
		k0, k1 = k1, k0
	}
	for name, entries := range map[string][]*kvfile.IndexEntry{
		"duplicate": {{Key: k0, Offset: 0, Size: 5}, {Key: k0, Offset: 5, Size: 5}},
		"empty key": {{Key: nil, Offset: 0, Size: 5}},
		"bad key":   {{Key: []byte("not a hash"), Offset: 0, Size: 5}},
		"past tail": {{Key: k0, Offset: 90, Size: 11}},
		"overflow":  {{Key: k0, Offset: 90, Size: ^uint64(0)}},
		"unsorted":  {{Key: k1, Offset: 0, Size: 5}, {Key: k0, Offset: 5, Size: 5}},
		"valid":     sortedEntries([]*kvfile.IndexEntry{{Key: k0, Offset: 0, Size: 5}, {Key: k1, Offset: 5, Size: 5}}),
	} {
		data := craftedPack(t, 100, entries)
		tail := mustReadIndexTail(t, data)
		ok := requireSameVerdict(t, tail, int64(len(data)), uint64(len(entries)), name)
		if ok != (name == "valid") {
			t.Fatalf("%s: built = %v", name, ok)
		}
	}
	// An empty pack has an empty tail, and both indexes accept it as empty.
	for _, tail := range [][]byte{nil, {}} {
		if !requireSameVerdict(t, tail, 0, 0, "empty pack") {
			t.Fatal("empty pack: rejected")
		}
	}
}

// TestTailIndexMatchesParsedIndexOnPackDir compares both indexes on every
// packfile in the directory named by PACKFILE_INDEX_PACK_DIR.
func TestTailIndexMatchesParsedIndexOnPackDir(t *testing.T) {
	dir := os.Getenv(indexPackDirEnv)
	if dir == "" {
		t.Skipf("%s is not set", indexPackDirEnv)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no packs in %s: %v", dir, err)
	}
	var keys int
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		fi, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		_, tail, err := kvfile.ReadIndexTail(f, uint64(fi.Size()))
		f.Close()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		rd, err := kvfile.BuildReaderWithIndexTail(tail, uint64(fi.Size()))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !requireSameVerdict(t, tail, fi.Size(), rd.Size(), name) {
			t.Fatalf("%s: tail rejected", name)
		}
		keys += int(rd.Size())
	}
	t.Logf("compared %d packs, %d keys", len(files), keys)
}

// TestPackfileStoreLookupStats counts candidate, opened and negative packs and
// index loads, then drops and reloads indexes under an index byte cap, with the
// in-place and the parsed index layouts.
func TestPackfileStoreLookupStats(t *testing.T) {
	for _, parsed := range []bool{false, true} {
		t.Run(map[bool]string{false: "in-place", true: "parsed"}[parsed], func(t *testing.T) {
			testPackfileStoreLookupStats(t, parsed)
		})
	}
}

// testPackfileStoreLookupStats checks lookup, index-cache, and eviction accounting for either index layout.
func testPackfileStoreLookupStats(t *testing.T, parsed bool) {
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
	store := NewPackfileStore(opener, newMemIndexCache(), &Config{ParsedIndex: &parsed, IndexByteLimit: 1})

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
	alphaHash := testPayloadHash(t, []byte("alpha"))
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

	// The constructor's index cap drops the colder index after both loads.
	if stats := store.SnapshotStats(); stats.IndexDrops != 1 {
		t.Fatalf("IndexDrops = %d, want 1", stats.IndexDrops)
	}

	// Read again: each probe reloads its dropped index from the index cache.
	_, found, err = store.GetBlock(ctx, &block.BlockRef{Hash: alphaHash})
	if err != nil || !found {
		t.Fatalf("GetBlock under the index cap: found=%v err=%v", found, err)
	}
	stats = store.SnapshotStats()
	if stats.IndexCacheHits != 2 || stats.RemoteIndexLoads != 2 {
		t.Fatalf("index cache hits=%d remote loads=%d, want 2/2", stats.IndexCacheHits, stats.RemoteIndexLoads)
	}
}

// TestPackfileStoreIndexCacheErrorStats counts index-cache errors while retaining remote reads.
func TestPackfileStoreIndexCacheErrorStats(t *testing.T) {
	// Publish a pack with an index cache that fails reads and writes.
	ctx := t.Context()
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})
	opener, _ := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, &errorIndexCache{
		getErr: errors.New("cache read failed"),
		setErr: errors.New("cache write failed"),
	}, nil)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "error-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})

	// Verify the target remains readable despite index cache failures.
	alphaHash := testPayloadHash(t, []byte("alpha"))
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

// TestPackfileStoreIndexCacheHitStats counts index tails served from cache.
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
	store := NewPackfileStore(opener, cache, nil)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "hit-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})

	// Read the target using the cached pack index.
	alphaHash := testPayloadHash(t, []byte("alpha"))
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

// TestPackfileStoreRejectsStaleIndexTailCache reloads an index that does not match the pack.
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
	store := NewPackfileStore(opener, cache, nil)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "stale-pack",
		BloomFilter: bloomBytes,
		BlockCount:  2,
		SizeBytes:   uint64(len(packBytes)),
	}})

	// Verify stale cache fallback finds the missing beta block.
	betaHash := testPayloadHash(t, []byte("beta"))
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

// TestPackfileStoreColdIndexTailFetchIsBounded bounds transport reads while locating a cold index tail.
func TestPackfileStoreColdIndexTailFetchIsBounded(t *testing.T) {
	// Build a pack containing one large target block.
	ctx := t.Context()
	large := bytes.Repeat([]byte("a"), 2<<20)
	packBytes, bloomBytes := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", string(large)}})

	// Publish the large pack with target-only fetch alignment.
	opener, transport := openerFromBytes(packBytes)
	store := NewPackfileStore(opener, newMemIndexCache(), nil)
	store.UpdateManifest([]*packfile.PackfileEntry{{
		Id:          "bounded-tail-pack",
		BloomFilter: bloomBytes,
		BlockCount:  1,
		SizeBytes:   uint64(len(packBytes)),
	}})
	store.SetWriteback(ctx, nil, 0)

	// Read the large block and verify its payload.
	h := testPayloadHash(t, large)
	got, found, err := store.GetBlock(ctx, &block.BlockRef{Hash: h})
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !found || !bytes.Equal(got, large) {
		t.Fatalf("expected large block, found=%v len=%d", found, len(got))
	}

	// Verify the first transport request is a bounded index suffix.
	first := transport.callAt(0)
	if first.length > packfile.IndexTailFirstRead {
		t.Fatalf("first fetch length = %d, want <= %d", first.length, packfile.IndexTailFirstRead)
	}
	if first.off < int64(len(packBytes)-packfile.IndexTailFirstRead) {
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

// TestPackfileStoreCachedTailDrivesCoBlockWriteback uses cached extents to select neighboring blocks.
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
	store := NewPackfileStore(opener, cache, nil)
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
	alphaHash := testPayloadHash(t, []byte("alpha"))
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

// TestPackfileStoreReopenReusesRawTailCache loads a new reader from the previous reader's cached tail.
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
	alphaHash := testPayloadHash(t, []byte("alpha"))

	// Read the target through the first store and verify its remote index load.
	firstOpener, firstTransport := openerFromBytes(packBytes)
	firstStore := NewPackfileStore(firstOpener, cache, nil)
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
	secondStore := NewPackfileStore(secondOpener, cache, nil)
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
	h := testPayloadHash(t, []byte("alpha"))
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
