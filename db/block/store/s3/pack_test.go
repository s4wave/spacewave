package block_store_s3

import (
	"slices"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// newTestPackStore opens a PackStore on prefix p/ of the fake bucket until the
// test ends.
func newTestPackStore(t *testing.T, client *Client) *PackStore {
	t.Helper()
	store := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", nil)
	t.Cleanup(store.Close)
	return store
}

// TestPackStoreBatch writes a batch as one packfile and reads each block back
// with its refs, from the writing store and from a store that lists the
// bucket.
func TestPackStoreBatch(t *testing.T) {
	// Open a writing pack store on the fake S3 bucket.
	ctx := t.Context()
	bucket, client := newFakeBucket(t, nil)
	writer := newTestPackStore(t, client)

	// Build the child and root block references for the packfile batch.
	child := []byte("child")
	childRef, err := block.BuildBlockRef(child, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := []byte("root")
	rootRef, err := block.BuildBlockRef(root, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Write the root and child together with their reference edge.
	err = writer.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: childRef, Data: child},
		{Ref: rootRef, Data: root, Refs: []*block.BlockRef{childRef}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the batch writes one packfile object and its entry.
	if bucket.puts != 2 {
		t.Fatalf("puts = %d; want the packfile and its entry", bucket.puts)
	}
	keys := bucket.keys()
	if len(keys) != 2 || !strings.HasPrefix(keys[0], "p/"+entryDir) || !strings.HasPrefix(keys[1], "p/"+packDir) {
		t.Fatalf("keys = %v; want one entry and one packfile", keys)
	}

	// Read the batch through both the writing store and a fresh pack store.
	reader := newTestPackStore(t, client)
	for _, store := range []*PackStore{writer, reader} {
		// Verify the stored root retains its bytes and child reference.
		stored, err := store.GetStoredBlock(ctx, rootRef)
		if err != nil {
			t.Fatal(err)
		}
		if stored == nil || string(stored.Data) != "root" || !stored.RefsKnown ||
			len(stored.Refs) != 1 || !stored.Refs[0].EqualsRef(childRef) {
			t.Fatalf("stored root = %v; want its data and the child ref", stored)
		}

		// Verify the batch existence probe finds both packed blocks.
		exists, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{childRef, rootRef})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(exists, []bool{true, true}) {
			t.Fatalf("exists = %v; want both blocks", exists)
		}
	}

	// Verify a pack store lookup returns no stored block for a missing reference.
	missing, err := block.BuildBlockRef([]byte("missing"), nil)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := reader.GetStoredBlock(ctx, missing)
	if err != nil || stored != nil {
		t.Fatalf("missing block = %v, %v; want nil", stored, err)
	}
}

// TestPackStoreFindsLaterPacks finds a packfile another store wrote after the
// first listing.
func TestPackStoreFindsLaterPacks(t *testing.T) {
	// Open independent writing and reading stores on the fake bucket.
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	reader := newTestPackStore(t, client)
	writer := newTestPackStore(t, client)

	// Write the first packfile and populate the reader manifest.
	first, _, err := writer.PutBlock(ctx, []byte("first"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if exists, err := reader.GetBlockExists(ctx, first); err != nil || !exists {
		t.Fatalf("first exists = %v, %v", exists, err)
	}

	// Write a later packfile and verify the reader discovers its block.
	second, _, err := writer.PutBlock(ctx, []byte("second"), nil)
	if err != nil {
		t.Fatal(err)
	}
	data, found, err := reader.GetBlock(ctx, second)
	if err != nil || !found || string(data) != "second" {
		t.Fatalf("second = %q, %v, %v", data, found, err)
	}
}
