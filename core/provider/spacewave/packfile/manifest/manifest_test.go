package manifest

import (
	"bytes"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/net/hash"

	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
)

// newTestStore creates an in-memory kvtx.Store for testing.
func newTestStore() kvtx.Store {
	return hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
}

// TestManifest tests ApplyDelta, GetEntries ordering, and
// GetLastPullSequence.
func TestManifest(t *testing.T) {
	ctx := t.Context()
	store := newTestStore()

	m, err := New(ctx, store)
	if err != nil {
		t.Fatal(err)
	}

	// Empty manifest initially.
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

	// Apply first delta.
	entries1 := []*packfile.PackfileEntry{
		{Id: "01ARZ3NDEKTSV4RRFFQ69G5FAV", BloomFilter: []byte("bf-1"), BloomFormatVersion: packfile.BloomFormatVersionV1, BlockCount: 10, SizeBytes: 1000, Sequence: 1},
		{Id: "01ARZ3NDEKTSV4RRFFQ69G5FAW", BloomFilter: []byte("bf-2"), BloomFormatVersion: packfile.BloomFormatVersionV1, BlockCount: 20, SizeBytes: 2000, Sequence: 2},
	}
	if err := m.ApplyDelta(ctx, entries1); err != nil {
		t.Fatal(err)
	}

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

	lastSeq, err = m.GetLastPullSequence(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lastSeq != 2 {
		t.Fatalf("expected last pull sequence 2, got %d", lastSeq)
	}

	// Apply second delta (appends).
	entries2 := []*packfile.PackfileEntry{
		{Id: "01ARZ3NDEKTSV4RRFFQ69G5FAX", BloomFilter: []byte("bf-3"), BlockCount: 5, SizeBytes: 500, Sequence: 3},
	}
	if err := m.ApplyDelta(ctx, entries2); err != nil {
		t.Fatal(err)
	}

	got = m.GetEntries()
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}

	lastSeq, err = m.GetLastPullSequence(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lastSeq != 3 {
		t.Fatalf("expected last pull sequence 3, got %d", lastSeq)
	}

	// Apply empty delta (no-op).
	if err := m.ApplyDelta(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if len(m.GetEntries()) != 3 {
		t.Fatal("empty delta should not change entries")
	}

	// Verify persistence: reload from same store.
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

	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

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

func TestManifestLoadsLegacyInlineBloomFilter(t *testing.T) {
	ctx := t.Context()
	store := newTestStore()

	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	entry := &packfile.PackfileEntry{
		Id:          "01ARZ3NDEKTSV4RRFFQ69G5FAY",
		BloomFilter: []byte("legacy-bloom"),
		BlockCount:  1,
		SizeBytes:   42,
	}
	data, err := entry.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, manifestPackKey(entry.GetId()), data); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, metaLastPullSequenceKey, []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	m, err := New(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	got := m.GetEntries()
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if string(got[0].GetBloomFilter()) != "legacy-bloom" {
		t.Fatalf("unexpected legacy bloom filter: %q", got[0].GetBloomFilter())
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
	_, err = writer.PackBlocks(&buf, func() (*hash.Hash, []byte, error) {
		if called {
			return nil, nil, nil
		}
		called = true
		return h, testData, nil
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
