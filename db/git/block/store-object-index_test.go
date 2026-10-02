package git_block

import (
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
)

// TestStorageHasEncodedObjectReadsOnlyIndex checks membership without opening pack data.
func TestStorageHasEncodedObjectReadsOnlyIndex(t *testing.T) {
	// Persist a pack in the real block-backed store.
	_, _, store := newPackfileTestStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	pack, hash := buildTestPackfile(t, []byte("indexed object"))
	writer, err := store.PackfileWriter()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(pack); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	// Query both present and absent hashes through pack indexes.
	if err := store.HasEncodedObject(hash); err != nil {
		t.Fatalf("indexed object absent: %v", err)
	}
	missing := plumbing.NewHash("1111111111111111111111111111111111111111")
	if err := store.HasEncodedObject(missing); err != plumbing.ErrObjectNotFound {
		t.Fatalf("missing object presence: %v", err)
	}

	// Membership must decode an index without opening any pack data reader.
	if len(store.packCache) != 1 {
		t.Fatalf("decoded indexes=%d, want 1", len(store.packCache))
	}
	for _, entry := range store.packCache {
		if entry.pack != nil {
			t.Fatal("membership opened pack data")
		}
	}
	if store.packLRU.Len() != 0 {
		t.Fatal("membership retained a pack reader")
	}
}
