package block_store_s3

import (
	"context"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// TestPackStoreKeepsPacks writes 200 one-block packfiles into a store that
// keeps its packs, runs compaction and reclaim on it, and finds every
// packfile and every version still in the bucket.
func TestPackStoreKeepsPacks(t *testing.T) {
	// Write 200 one-block packfiles into a versioned bucket.
	ctx := t.Context()
	bucket, client := newFakeBucket(t, nil)
	bucket.versioned = true
	store := NewPackStore(logrus.NewEntry(logrus.New()), client, "bucket", "p/", &PackStoreOptions{KeepPacks: true})
	t.Cleanup(store.Close)
	refs := putBlocks(t, store, "block", 200)

	// Compaction merges nothing, and reclaim judges nothing.
	if err := store.compact(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := store.Reclaim(ctx, func(context.Context) error {
		t.Fatal("reclaim called its fence")
		return nil
	}, func(context.Context, []*block.BlockRef) ([]bool, error) {
		t.Fatal("reclaim judged liveness")
		return nil, nil
	})
	if err != nil || !next.IsZero() {
		t.Fatalf("Reclaim = %v, %v; want no pass due", next, err)
	}

	// Every packfile and entry remains, and no version was deleted.
	var packs, entries int
	for _, key := range bucket.keys() {
		switch {
		case strings.HasPrefix(key, "p/"+packDir):
			packs++
		case strings.HasPrefix(key, "p/"+entryDir):
			entries++
		}
	}
	if packs != 200 || entries != 200 || bucket.hiddenVersions() != 0 {
		t.Fatalf("bucket holds %d packfiles, %d entries and %d hidden versions; want 200, 200 and 0", packs, entries, bucket.hiddenVersions())
	}

	// A fresh store reads every block back.
	checkBlocks(t, newTestPackStore(t, client), "block", refs)
}
