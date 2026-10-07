package store

import (
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// TestManifestDeltaLookupAndReaders keeps ordered bloomless lookup and unaffected readers.
func TestManifestDeltaLookupAndReaders(t *testing.T) {
	// Build a block and its bloomless pack for deterministic lookup.
	data, _ := buildTestPack(t, map[string][]byte{"block": []byte("payload")})
	ref, err := block.BuildBlockRef([]byte("payload"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Open three bloomless packs containing the same block.
	var opened []string
	store := NewPackfileStore(func(id string, size int64) (*PackReader, error) {
		opened = append(opened, id)
		return NewPackReader(id, size, &bytesTransport{data: data}), nil
	}, nil)
	t.Cleanup(store.Close)
	entries := []*packfile.PackfileEntry{
		{Id: "local", SizeBytes: uint64(len(data)), BlockCount: 1},
		{Id: "server-b", Sequence: 7, SizeBytes: uint64(len(data)), BlockCount: 1},
		{Id: "server-a", Sequence: 7, SizeBytes: uint64(len(data)), BlockCount: 1},
	}
	store.ApplyManifestDelta(entries, nil)

	// Verify the first server pack wins the sequence and ID ordering.
	read := func() {
		t.Helper()
		if got, found, err := store.GetBlock(t.Context(), ref); err != nil || !found || string(got) != "payload" {
			t.Fatalf("bloomless read: data=%q found=%v err=%v", got, found, err)
		}
	}
	read()
	if !slices.Equal(opened, []string{"server-a"}) {
		t.Fatalf("lookup order = %v", opened)
	}
	reader := store.engines["server-a"]

	// Replay and unrelated insertion retain the opened server reader.
	store.ApplyManifestDelta(entries, nil)
	store.ApplyManifestDelta([]*packfile.PackfileEntry{{Id: "unrelated"}}, nil)
	read()
	if store.engines["server-a"] != reader || len(opened) != 1 {
		t.Fatal("replay or unrelated add evicted an accepted reader")
	}

	// A sequence update reorders lookup without retiring the immutable reader.
	newest := entries[1].CloneVT()
	newest.Sequence = 8
	store.ApplyManifestDelta([]*packfile.PackfileEntry{newest}, nil)
	read()
	if !slices.Equal(opened, []string{"server-a", "server-b"}) {
		t.Fatalf("newest pack was not selected: %v", opened)
	}
	if store.engines["server-a"] != reader {
		t.Fatal("unrelated sequence update evicted server-a")
	}

	// Changed contents evict only the affected reader; deletion exposes the local pack.
	changed := newest.CloneVT()
	changed.SizeBytes++
	store.ApplyManifestDelta([]*packfile.PackfileEntry{changed}, nil)
	if store.engines["server-b"] != nil || store.engines["server-a"] != reader {
		t.Fatal("content update evicted the wrong readers")
	}
	store.ApplyManifestDelta(nil, []string{"server-a", "server-b", "unrelated"})
	read()
	if !slices.Equal(opened, []string{"server-a", "server-b", "local"}) {
		t.Fatalf("deletion failed to expose local pack: %v", opened)
	}
}
