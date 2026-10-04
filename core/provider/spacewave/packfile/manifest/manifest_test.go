package manifest

import (
	"bytes"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	"github.com/s4wave/spacewave/net/hash"

	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

// newTestStore creates an in-memory kvtx.Store for testing.
func newTestStore() kvtx.Store {
	return hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
}

// TestManifestApplyDeltaKeepsPulledSequence verifies a locally authored entry
// committed after a pull of the same pack keeps the pulled sequence.
func TestManifestApplyDeltaKeepsPulledSequence(t *testing.T) {
	// Open an empty manifest.
	ctx := t.Context()
	store := newTestStore()
	m, err := New(ctx, store)
	if err != nil {
		t.Fatal(err)
	}

	// Pull pack-a at sequence 7, then commit the same pack locally.
	if err := m.ApplyDelta(ctx, []*packfile.PackfileEntry{{Id: "pack-a", BlockCount: 1, Sequence: 7}}, nil, 7); err != nil {
		t.Fatal(err)
	}
	if err := m.ApplyDelta(ctx, []*packfile.PackfileEntry{{Id: "pack-a", BlockCount: 1}}, nil, 0); err != nil {
		t.Fatal(err)
	}

	// The pulled sequence survives in memory and in the store.
	if got := m.GetEntries()[0].GetSequence(); got != 7 {
		t.Fatalf("sequence = %d, want the pulled 7", got)
	}
	reloaded, err := New(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.GetEntries()[0].GetSequence(); got != 7 {
		t.Fatalf("stored sequence = %d, want the pulled 7", got)
	}
}

// TestManifest tests ApplyDelta, GetEntries ordering, GetLastPullSequence, and
// the stored key layout.
func TestManifest(t *testing.T) {
	// Open an empty manifest with no pull cursor.
	ctx := t.Context()
	store := newTestStore()
	m, err := New(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.GetEntries()) != 0 {
		t.Fatal("expected empty manifest")
	}
	lastSeq, err := m.GetLastPullSequence(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lastSeq != 0 {
		t.Fatalf("expected empty last pull sequence, got %d", lastSeq)
	}

	// Apply the first delta.
	entries1 := []*packfile.PackfileEntry{
		{Id: "01ARZ3NDEKTSV4RRFFQ69G5FAV", BloomFilter: []byte("bf-1"), BloomFormatVersion: packfile.BloomFormatVersionV1, BlockCount: 10, SizeBytes: 1000, Sequence: 1},
		{Id: "01ARZ3NDEKTSV4RRFFQ69G5FAW", BloomFilter: []byte("bf-2"), BloomFormatVersion: packfile.BloomFormatVersionV1, BlockCount: 20, SizeBytes: 2000, Sequence: 2},
	}
	if err := m.ApplyDelta(ctx, entries1, nil, 2); err != nil {
		t.Fatal(err)
	}

	// The entries are listed in order with their bloom filters.
	got := m.GetEntries()
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].GetId() != "01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Fatalf("unexpected first entry ID: %s", got[0].GetId())
	}
	if string(got[0].GetBloomFilter()) != "bf-1" {
		t.Fatalf("unexpected first entry bloom filter: %q", got[0].GetBloomFilter())
	}

	// The cursor advances to the delta's cursor.
	lastSeq, err = m.GetLastPullSequence(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lastSeq != 2 {
		t.Fatalf("expected last pull sequence 2, got %d", lastSeq)
	}

	// A second delta appends an entry and advances the cursor.
	entries2 := []*packfile.PackfileEntry{
		{Id: "01ARZ3NDEKTSV4RRFFQ69G5FAX", BloomFilter: []byte("bf-3"), BlockCount: 5, SizeBytes: 500, Sequence: 3},
	}
	if err := m.ApplyDelta(ctx, entries2, nil, 3); err != nil {
		t.Fatal(err)
	}
	if got = m.GetEntries(); len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	lastSeq, err = m.GetLastPullSequence(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lastSeq != 3 {
		t.Fatalf("expected last pull sequence 3, got %d", lastSeq)
	}

	// An empty delta changes nothing.
	if err := m.ApplyDelta(ctx, nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	if len(m.GetEntries()) != 3 {
		t.Fatal("empty delta should not change entries")
	}

	// A reload from the same store lists the same entries.
	m2, err := New(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	got2 := m2.GetEntries()
	if len(got2) != 3 {
		t.Fatalf("expected 3 entries after reload, got %d", len(got2))
	}
	if got2[0].GetId() != "01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Fatalf("unexpected first reloaded entry ID: %s", got2[0].GetId())
	}
	if string(got2[0].GetBloomFilter()) != "bf-1" {
		t.Fatalf("unexpected first reloaded bloom filter: %q", got2[0].GetBloomFilter())
	}

	// Open a read transaction to inspect the stored layout.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Each pack is stored under its sharded key.
	var keys []string
	if err := tx.ScanPrefix(ctx, []byte("packs/"), func(key, value []byte) error {
		keys = append(keys, string(key))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 stored pack keys, got %d", len(keys))
	}
	if keys[0] != "packs/AV/01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Fatalf("unexpected first stored pack key: %s", keys[0])
	}

	// The stored entry omits its bloom filter and keeps the format version.
	data, found, err := tx.Get(ctx, manifestPackKey("01ARZ3NDEKTSV4RRFFQ69G5FAV"))
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected persisted pack entry")
	}
	storedEntry := &packfile.PackfileEntry{}
	if err := storedEntry.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if len(storedEntry.GetBloomFilter()) != 0 {
		t.Fatalf("expected persisted pack entry bloom filter to be split out, got %d bytes", len(storedEntry.GetBloomFilter()))
	}
	if storedEntry.GetBloomFormatVersion() != packfile.BloomFormatVersionV1 {
		t.Fatalf("expected persisted bloom_format_version v1, got %d", storedEntry.GetBloomFormatVersion())
	}

	// The bloom filter is stored under its own key.
	bloomData, found, err := tx.Get(ctx, manifestBloomKey("01ARZ3NDEKTSV4RRFFQ69G5FAV"))
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected persisted bloom filter")
	}
	if string(bloomData) != "bf-1" {
		t.Fatalf("unexpected persisted bloom filter: %q", bloomData)
	}
}

// TestManifestApplyDeltaUpdatesByIDAndAppliesReplacementEvents verifies a
// delta updates entries by id and a replacement event drops the replaced pack
// and its cached index tail.
func TestManifestApplyDeltaUpdatesByIDAndAppliesReplacementEvents(t *testing.T) {
	// Open an empty manifest.
	ctx := t.Context()
	store := newTestStore()
	m, err := New(ctx, store)
	if err != nil {
		t.Fatal(err)
	}

	// Pull pack-a and pack-b, and cache pack-a's index tail.
	if err := m.ApplyDelta(ctx, []*packfile.PackfileEntry{
		{Id: "pack-a", BloomFilter: []byte("bf-a"), BloomFormatVersion: packfile.BloomFormatVersionV1, BlockCount: 1, SizeBytes: 10, Sequence: 1},
		{Id: "pack-b", BloomFilter: []byte("bf-b"), BloomFormatVersion: packfile.BloomFormatVersionV1, BlockCount: 1, SizeBytes: 20, Sequence: 2},
	}, nil, 2); err != nil {
		t.Fatal(err)
	}
	if err := NewIndexCache(store).Set(ctx, "pack-a", []byte("tail-a")); err != nil {
		t.Fatal(err)
	}

	// Update pack-b, add pack-c, and replace pack-a.
	if err := m.ApplyDelta(ctx, []*packfile.PackfileEntry{
		{Id: "pack-b", BloomFilter: []byte("bf-b2"), BloomFormatVersion: packfile.BloomFormatVersionV1, BlockCount: 2, SizeBytes: 30, Sequence: 3},
		{Id: "pack-c", BloomFilter: []byte("bf-c"), BloomFormatVersion: packfile.BloomFormatVersionV1, BlockCount: 3, SizeBytes: 40, Sequence: 4},
	}, []*packfile.PackReplacementEvent{{
		Sequence:           5,
		ReplacedPackIds:    []string{"pack-a"},
		ReplacementPackIds: []string{"pack-c"},
	}}, 5); err != nil {
		t.Fatal(err)
	}

	// The active entries are the updated pack-b and pack-c.
	got := m.GetEntries()
	if len(got) != 2 {
		t.Fatalf("expected 2 active entries, got %d", len(got))
	}
	if got[0].GetId() != "pack-b" || got[0].GetBlockCount() != 2 {
		t.Fatalf("pack-b was not updated: %+v", got[0])
	}
	if got[1].GetId() != "pack-c" {
		t.Fatalf("pack-c missing: %+v", got[1])
	}

	// The cursor advances to the event sequence.
	lastSeq, err := m.GetLastPullSequence(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lastSeq != 5 {
		t.Fatalf("last sequence=%d want 5", lastSeq)
	}

	// A reload lists the same active entries.
	reloaded, err := New(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	got = reloaded.GetEntries()
	if len(got) != 2 || got[0].GetId() != "pack-b" || got[1].GetId() != "pack-c" {
		t.Fatalf("unexpected reloaded active entries: %+v", got)
	}

	// Open a read transaction to inspect the stored keys.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Neither pack-a's entry nor its index tail remains.
	_, found, err := tx.Get(ctx, manifestPackKey("pack-a"))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("replaced pack-a still persisted")
	}
	_, found, err = tx.Get(ctx, indexCacheKey("pack-a"))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("replaced pack-a index tail still cached")
	}
}

// TestManifestApplyDeltaReplaysIdentically verifies a delta replayed after a
// fault before commit stores the same result as a clean apply.
func TestManifestApplyDeltaReplaysIdentically(t *testing.T) {
	// result is the stored catalog after a run.
	type result struct {
		entries  []*packfile.PackfileEntry
		sequence uint64
	}

	// run applies two deltas and reloads the stored catalog.
	run := func(t *testing.T, injectFault bool) (result, *kvtest.FaultStore) {
		// Open a manifest holding pack-a and pack-b.
		t.Helper()
		ctx := t.Context()
		backend := newTestStore()
		m, err := New(ctx, backend)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.ApplyDelta(ctx, []*packfile.PackfileEntry{
			{Id: "pack-a", BloomFilter: []byte("bf-a"), BlockCount: 1, Sequence: 1},
			{Id: "pack-b", BloomFilter: []byte("bf-b"), BlockCount: 2, Sequence: 2},
		}, nil, 2); err != nil {
			t.Fatal(err)
		}

		// Apply the second delta, optionally through a store that faults first.
		var faultStore *kvtest.FaultStore
		if injectFault {
			faultStore = kvtest.NewFaultStore(backend, kvtest.FaultBeforeCommit)
			m.store = faultStore
		}
		if err := m.ApplyDelta(ctx, []*packfile.PackfileEntry{
			{Id: "pack-b", BloomFilter: []byte("bf-b-updated"), BlockCount: 3, Sequence: 3},
			{Id: "pack-c", BloomFilter: []byte("bf-c"), BlockCount: 4, Sequence: 4},
		}, []*packfile.PackReplacementEvent{{
			Sequence:        5,
			ReplacedPackIds: []string{"pack-a"},
		}}, 5); err != nil {
			t.Fatal(err)
		}

		// Reload the stored result.
		reloaded, err := New(ctx, backend)
		if err != nil {
			t.Fatal(err)
		}
		sequence, err := reloaded.GetLastPullSequence(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return result{entries: reloaded.GetEntries(), sequence: sequence}, faultStore
	}

	// The faulted run stores the clean run's result.
	want, _ := run(t, false)
	got, faultStore := run(t, true)
	if got.sequence != want.sequence {
		t.Fatalf("last sequence = %d, want %d", got.sequence, want.sequence)
	}
	if len(got.entries) != len(want.entries) {
		t.Fatalf("entry count = %d, want %d", len(got.entries), len(want.entries))
	}
	for i := range want.entries {
		if !got.entries[i].EqualVT(want.entries[i]) {
			t.Fatalf("entry %d = %+v, want %+v", i, got.entries[i], want.entries[i])
		}
	}

	// The fault retried once and committed once.
	if got := faultStore.Opened(); got != 2 {
		t.Fatalf("opened transactions = %d, want 2", got)
	}
	if got := faultStore.DelegatedCommits(); got != 1 {
		t.Fatalf("delegated commits = %d, want 1", got)
	}
}

// TestIndexCache tests Get/Set round-trip for raw index-tail bytes.
func TestIndexCache(t *testing.T) {
	ctx := t.Context()
	store := newTestStore()

	cache := NewIndexCache(store)

	// Get from empty cache.
	_, ok, err := cache.Get(ctx, "pack-001")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected cache miss")
	}

	// Build some test index entries via a packfile.
	var buf bytes.Buffer
	testData := []byte("test-block-data")
	h, err := hash.Sum(hash.HashType_HashType_SHA256, testData)
	if err != nil {
		t.Fatal(err)
	}

	called := false
	_, err = writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if called {
			return nil, nil, nil
		}
		called = true
		return h, &block.StoredBlock{Data: testData, RefsKnown: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	_, tail, err := kvfile.ReadIndexTail(bytes.NewReader(buf.Bytes()), uint64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) == 0 {
		t.Fatal("expected non-empty raw index tail")
	}

	// Set and get.
	if err := cache.Set(ctx, "pack-001", tail); err != nil {
		t.Fatal(err)
	}
	got, ok, err := cache.Get(ctx, "pack-001")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !bytes.Equal(got, tail) {
		t.Fatal("raw tail mismatch after round-trip")
	}

	// Different pack ID should miss.
	_, ok, err = cache.Get(ctx, "pack-002")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected cache miss for different pack ID")
	}

	// Verify persistence.
	cache2 := NewIndexCache(store)
	got2, ok, err := cache2.Get(ctx, "pack-001")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected cache hit after new IndexCache instance")
	}
	if !bytes.Equal(got2, tail) {
		t.Fatal("raw tail mismatch after reload")
	}
}
