package block_store_s3

import (
	"bytes"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
	"golang.org/x/sync/errgroup"
)

// TestPackStoreCompacts merges 200 one-block packfiles into a few without
// listing the bucket, and a reader that listed the packfiles before the merge
// still reads every block.
func TestPackStoreCompacts(t *testing.T) {

	// Write 200 one-block packfiles into a fresh fake bucket.
	ctx := t.Context()
	bucket, client := newFakeBucket(t, nil)
	writer := newTestPackStore(t, client)
	refs := putBlocks(t, writer, "block", 200)

	// Record the pre-compaction listing in a reader store and check a missing block.
	reader := newTestPackStore(t, client)
	missing, err := block.BuildBlockRef([]byte("missing"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if exists, err := reader.GetBlockExists(ctx, missing); err != nil || exists {
		t.Fatalf("missing exists = %v, %v", exists, err)
	}

	// Compact the writer store, which knows every packfile it wrote.
	lists := bucket.lists
	if err := writer.compact(ctx); err != nil {
		t.Fatal(err)
	}
	if bucket.lists != lists {
		t.Fatalf("compaction listed the bucket %d times", bucket.lists-lists)
	}

	// Count the remaining packfiles and entries.
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

	// Read every block back through a stale reader and a fresh store.
	for _, store := range []*PackStore{reader, newTestPackStore(t, client)} {
		checkBlocks(t, store, "block", refs)
	}

	// A duplicate with unknown refs cannot erase a recorded list, and two
	// listed copies keep the union of their lists, in either input order, so
	// a merge that ranks an old short copy later keeps the child a corrected
	// copy names. The merged pack still holds exactly one copy.
	type copyRefs struct {
		known    bool
		children []int
	}
	cases := []struct {
		name   string
		copies [2]copyRefs
		want   []int
	}{
		{"known-first", [2]copyRefs{{true, []int{0}}, {false, nil}}, []int{0}},
		{"known-last", [2]copyRefs{{false, nil}, {true, []int{0}}}, []int{0}},
		{"short-last", [2]copyRefs{{true, []int{0, 1}}, {true, []int{0}}}, []int{0, 1}},
		{"lists-differ", [2]copyRefs{{true, []int{1}}, {true, []int{0}}}, []int{1, 0}},
	}
	for _, tc := range cases {
		t.Run("duplicate-refs/"+tc.name, func(t *testing.T) {
			// Write two one-block packs with identical bytes and the case's refs.
			ctx := t.Context()
			_, client := newFakeBucket(t, nil)
			store := newTestPackStore(t, client)
			data := []byte("parent")
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			var children []*block.BlockRef
			for _, name := range []string{"child", "other"} {
				child, err := block.BuildBlockRef([]byte(name), nil)
				if err != nil {
					t.Fatal(err)
				}
				children = append(children, child)
			}
			var inputs []*packfile.PackfileEntry
			for _, c := range tc.copies {
				stored := &block.StoredBlock{Data: data, RefsSource: block.AssertedIf(c.known)}
				for _, i := range c.children {
					stored.Refs = append(stored.Refs, children[i])
				}
				entry, err := store.writePack(ctx, func() (*hash.Hash, *block.StoredBlock, error) {
					if stored == nil {
						return nil, nil, nil
					}
					next := stored
					stored = nil
					return ref.GetHash(), next, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				inputs = append(inputs, entry)
			}

			// Merge the packs and read the surviving copy through a fresh store.
			if err := store.merge(ctx, inputs); err != nil {
				t.Fatal(err)
			}
			fresh := newTestPackStore(t, client)
			stored, err := fresh.GetStoredBlock(ctx, ref)
			if err != nil || stored == nil || !bytes.Equal(stored.Data, data) ||
				stored.RefsSource != block.RefsListed || len(stored.Refs) != len(tc.want) {
				t.Fatalf("merged block = %v, %v; want its bytes and %d listed children", stored, err, len(tc.want))
			}
			for i, want := range tc.want {
				if !stored.Refs[i].EqualsRef(children[want]) {
					t.Fatalf("merged child %d = %s; want %s", i, stored.Refs[i].MarshalString(), children[want].MarshalString())
				}
			}

			// Count the actual surviving blocks, catching both loss and invention.
			var count int
			if err := fresh.ScanBlocks(ctx, func(got *block.BlockRef, _ *block.StoredBlock) error {
				if !got.EqualsRef(ref) {
					t.Fatalf("merged pack invented block %s", got.MarshalString())
				}
				count++
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("merged packs hold %d blocks; want 1", count)
			}
		})
	}
}

// TestPackStoreConcurrentCompaction runs compaction on two stores sharing the
// bucket at once and loses no block.
func TestPackStoreConcurrentCompaction(t *testing.T) {

	// Write blocks into two stores sharing one fake bucket.
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	a := newTestPackStore(t, client)
	b := newTestPackStore(t, client)
	aRefs := putBlocks(t, a, "a", 40)
	bRefs := putBlocks(t, b, "b", 40)

	// List the entries in both stores so their merges pick the same inputs.
	for _, store := range []*PackStore{a, b} {
		if err := store.listEntries(ctx, store.listings.Load()); err != nil {
			t.Fatal(err)
		}
	}

	// Compact both stores concurrently.
	var eg errgroup.Group
	eg.Go(func() error { return a.compact(ctx) })
	eg.Go(func() error { return b.compact(ctx) })
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}

	// Read every block of both stores back through a fresh reader.
	reader := newTestPackStore(t, client)
	checkBlocks(t, reader, "a", aRefs)
	checkBlocks(t, reader, "b", bRefs)
}

// putBlocks writes count one-block packfiles holding name and an index.
func putBlocks(t *testing.T, store *PackStore, name string, count int) []*block.BlockRef {
	t.Helper()

	// Put each block into the store and collect its references.
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

	// Read each block and compare its value to the original.
	for i, ref := range refs {
		data, found, err := store.GetBlock(t.Context(), ref)
		if err != nil || !found || string(data) != name+strconv.Itoa(i) {
			t.Fatalf("block %s%d = %q, %v, %v", name, i, data, found, err)
		}
	}
}

// TestPackStoreRewriteDuringReplace writes a block again while a replace of
// its first packfile has deleted the entry but not yet the packfile, and the
// block stays readable.
func TestPackStoreRewriteDuringReplace(t *testing.T) {
	// Write the block and note its packfile.
	ctx := t.Context()
	_, client := newFakeBucket(t, nil)
	store := newTestPackStore(t, client)
	refs := putBlocks(t, store, "block", 1)
	var first []string
	store.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		first = slices.Collect(maps.Keys(store.entries))
	})
	if len(first) != 1 {
		t.Fatalf("store holds %d packfiles; want 1", len(first))
	}

	// Run a replace of that packfile with the same block written between its
	// entry and packfile deletes.
	if err := store.deleteObjects(ctx, entryDir, first); err != nil {
		t.Fatal(err)
	}
	store.updateEntries(nil, first)
	putBlocks(t, store, "block", 1)
	if err := store.deleteObjects(ctx, packDir, first); err != nil {
		t.Fatal(err)
	}

	// Read the block back through a fresh store.
	checkBlocks(t, newTestPackStore(t, client), "block", refs)
}
