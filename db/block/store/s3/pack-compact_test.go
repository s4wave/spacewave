package block_store_s3

import (
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"golang.org/x/sync/errgroup"
)

// TestPackStoreCompacts merges 200 one-block packfiles into a few, and a
// reader that listed the packfiles before the merge still reads every block.
func TestPackStoreCompacts(t *testing.T) {
	ctx := t.Context()
	bucket, client := newFakeBucket(t, nil)
	writer := newTestPackStore(t, client)
	refs := putBlocks(t, writer, "block", 200)

	reader := newTestPackStore(t, client)
	missing, err := block.BuildBlockRef([]byte("missing"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if exists, err := reader.GetBlockExists(ctx, missing); err != nil || exists {
		t.Fatalf("missing exists = %v, %v", exists, err)
	}

	if err := writer.compact(ctx); err != nil {
		t.Fatal(err)
	}
	var packs, entries int
	for _, key := range bucket.keys() {
		switch {
		case strings.HasPrefix(key, "p/"+packDir):
			packs++
		case strings.HasPrefix(key, "p/"+entryDir):
			entries++
		}
	}
	// 200 packfiles of weight 1/4096 merge into 25 of 8 blocks, then 3 of 64
	// blocks, leaving one of 8.
	if packs != 4 || entries != 4 {
		t.Fatalf("bucket holds %d packfiles and %d entries; want 4 of each", packs, entries)
	}

	for _, store := range []*PackStore{reader, newTestPackStore(t, client)} {
		checkBlocks(t, store, "block", refs)
	}
}

// TestPackStoreConcurrentCompaction runs compaction on two stores sharing the
// bucket at once and loses no block.
func TestPackStoreConcurrentCompaction(t *testing.T) {
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	a := newTestPackStore(t, client)
	b := newTestPackStore(t, client)
	aRefs := putBlocks(t, a, "a", 40)
	bRefs := putBlocks(t, b, "b", 40)

	var eg errgroup.Group
	eg.Go(func() error { return a.compact(ctx) })
	eg.Go(func() error { return b.compact(ctx) })
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}

	reader := newTestPackStore(t, client)
	checkBlocks(t, reader, "a", aRefs)
	checkBlocks(t, reader, "b", bRefs)
}

// putBlocks writes count one-block packfiles holding name and an index.
func putBlocks(t *testing.T, store *PackStore, name string, count int) []*block.BlockRef {
	t.Helper()
	refs := make([]*block.BlockRef, count)
	for i := range refs {
		ref, _, err := store.PutBlock(t.Context(), []byte(name+strconv.Itoa(i)), nil)
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = ref
	}
	return refs
}

// checkBlocks reads each block putBlocks wrote from store.
func checkBlocks(t *testing.T, store *PackStore, name string, refs []*block.BlockRef) {
	t.Helper()
	for i, ref := range refs {
		data, found, err := store.GetBlock(t.Context(), ref)
		if err != nil || !found || string(data) != name+strconv.Itoa(i) {
			t.Fatalf("block %s%d = %q, %v, %v", name, i, data, found, err)
		}
	}
}
