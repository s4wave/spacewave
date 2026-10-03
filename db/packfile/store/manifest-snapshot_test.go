package store

import (
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// TestManifestSnapshotReadAfterRemoval completes an acquired lookup without
// reinstalling a removed pack's reader in the current catalog.
func TestManifestSnapshotReadAfterRemoval(t *testing.T) {
	// Build a pack and block reference for the snapshot removal check.
	data, bloom := buildTestPack(t, map[string][]byte{"block": []byte("payload")})
	ref, err := block.BuildBlockRef([]byte("payload"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Gate opening the pack reader while publishing the initial manifest.
	opening, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	store := NewPackfileStore(func(id string, size int64) (*PackReader, error) {
		once.Do(func() { close(opening); <-resume })
		return NewPackReader(id, size, &bytesTransport{data: data}), nil
	}, nil)
	t.Cleanup(func() { store.Close() })
	store.ApplyManifestDelta([]*packfile.PackfileEntry{{
		Id: "removed", SizeBytes: uint64(len(data)), BlockCount: 1, BloomFilter: bloom,
	}}, nil)

	// Start the block lookup through the acquired manifest snapshot.
	snapshot := store.SnapshotManifest()
	done := make(chan error, 1)
	go func() {
		found, err := snapshot.GetBlockExistsBatch(t.Context(), []*block.BlockRef{ref})
		if err == nil && (len(found) != 1 || !found[0]) {
			t.Error("acquired snapshot lost its pack")
		}
		done <- err
	}()

	// Remove the pack while its snapshot lookup is opening the reader.
	<-opening
	store.ApplyManifestDelta(nil, []string{"removed"})
	close(resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// Verify the current manifest retains neither the pack nor its reader.
	found, err := store.GetBlockExistsBatch(t.Context(), []*block.BlockRef{ref})
	if err != nil || len(found) != 1 || found[0] {
		t.Fatalf("current catalog retained removed pack: found=%v err=%v", found, err)
	}
	if count := store.SnapshotStats().EngineCount; count != 0 {
		t.Fatalf("removed pack reopened a cached reader: %d", count)
	}
}
