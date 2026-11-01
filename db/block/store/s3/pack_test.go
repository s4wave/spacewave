package block_store_s3

import (
	"slices"
	"strings"
	"testing"
	"time"

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
	_, err = writer.PutBlockBatch(ctx, []*block.PutBatchEntry{
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

// TestPackStoreFindsLaterPacks checks that a miss lists nothing and that the
// refresh finds a packfile another store wrote after the first listing.
func TestPackStoreFindsLaterPacks(t *testing.T) {
	// Open independent writing and reading stores on the fake bucket, the
	// reader refreshing every 20 ms.
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	reader := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{
		RefreshInterval: 20 * time.Millisecond,
	})
	t.Cleanup(reader.Close)
	writer := newTestPackStore(t, client)

	// Write the first packfile and populate the reader manifest.
	first, _, err := writer.PutBlock(ctx, []byte("first"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if exists, err := reader.GetBlockExists(ctx, first); err != nil || !exists {
		t.Fatalf("first exists = %v, %v", exists, err)
	}

	// Write a later packfile and look for its block until the refresh lists it.
	second, _, err := writer.PutBlock(ctx, []byte("second"), nil)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, found, err := reader.GetBlock(ctx, second)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			if string(data) != "second" {
				t.Fatalf("second = %q", data)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the refresh never found the later packfile")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestPackStoreMissDoesNotList checks that a lookup that finds no block reads
// the known packfiles alone: the store lists the entries once, whatever the
// number of misses.
func TestPackStoreMissDoesNotList(t *testing.T) {
	// Load the manifest with one lookup.
	ctx := t.Context()
	bucket, client := newFakeBucket(t, nil)
	store := newTestPackStore(t, client)
	missing, err := block.BuildBlockRef([]byte("missing"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if exists, err := store.GetBlockExists(ctx, missing); err != nil || exists {
		t.Fatalf("missing exists = %v, %v", exists, err)
	}
	listed := store.listings.Load()
	requests := bucket.listCount()

	// Miss again: no listing starts and the bucket sees no request.
	for range 3 {
		if exists, err := store.GetBlockExists(ctx, missing); err != nil || exists {
			t.Fatalf("missing exists = %v, %v", exists, err)
		}
	}
	if store.listings.Load() != listed || bucket.listCount() != requests {
		t.Fatalf("a miss listed: %d listings, %d requests", store.listings.Load()-listed, bucket.listCount()-requests)
	}
}
