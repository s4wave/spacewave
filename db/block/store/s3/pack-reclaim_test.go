package block_store_s3

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestPackStoreReclaim rewrites the listed packfiles that are at least half
// dead, deletes the ones with no live block, skips the ones another writer
// removed, and leaves the packfiles written after the fence and no hidden
// versions.
func TestPackStoreReclaim(t *testing.T) {
	// Serve a versioned bucket.
	ctx := t.Context()
	bucket, client := newFakeBucket(t, nil)
	bucket.versioned = true
	store := newTestPackStore(t, client)

	// Write the listed packfiles and choose the dead blocks.
	mostlyDead := putPack(t, bucket, store, "a0", "a1", "a2", "a3")
	mostlyLive := putPack(t, bucket, store, "b0", "b1", "b2", "b3")
	allDead := putPack(t, bucket, store, "c0")
	removed := putPack(t, bucket, store, "e0")
	dead := map[string]bool{"a0": true, "a1": true, "a2": true, "b0": true, "c0": true, "d0": true, "e0": true}

	// The fence writes a packfile after the listing and removes a listed one,
	// as another writer's merge would.
	var afterFence *testPack
	fences := 0
	fence := func(context.Context) error {
		// Count the fence and write a packfile after the listing.
		fences++
		afterFence = putPack(t, bucket, store, "d0")

		// Remove a listed packfile.
		bucket.mtx.Lock()
		delete(bucket.objects, "p/"+entryDir+removed.id)
		delete(bucket.objects, "p/"+packDir+removed.id)
		bucket.mtx.Unlock()
		return nil
	}

	// Report the dead blocks by name, and fail on any block not listed.
	names := make(map[string]string)
	for _, pack := range []*testPack{mostlyDead, mostlyLive, allDead, removed} {
		for i, ref := range pack.refs {
			names[ref.MarshalString()] = pack.names[i]
		}
	}
	live := func(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
		alive := make([]bool, len(refs))
		for i, ref := range refs {
			name, ok := names[ref.MarshalString()]
			if !ok {
				t.Errorf("reclaim judged block %s written after the fence", ref.MarshalString())
			}
			alive[i] = !dead[name]
		}
		return alive, nil
	}

	// Run one pass.
	if err := store.Reclaim(ctx, fence, live); err != nil {
		t.Fatal(err)
	}
	if fences != 1 {
		t.Fatalf("fence ran %d times", fences)
	}

	// Check the replaced packfiles are gone with no hidden versions.
	keys := bucket.keys()
	for _, pack := range []*testPack{mostlyDead, allDead} {
		if slices.Contains(keys, "p/"+packDir+pack.id) {
			t.Errorf("packfile %v remains", pack.names)
		}
	}
	for _, pack := range []*testPack{mostlyLive, afterFence} {
		if !slices.Contains(keys, "p/"+packDir+pack.id) {
			t.Errorf("packfile %v was replaced", pack.names)
		}
	}
	if hidden := bucket.hiddenVersions(); hidden != 0 {
		t.Errorf("bucket keeps %d hidden versions", hidden)
	}

	// Read every block back through a new store.
	reader := newTestPackStore(t, client)
	for _, pack := range []*testPack{mostlyDead, mostlyLive, allDead, afterFence} {
		for i, ref := range pack.refs {
			name := pack.names[i]
			data, found, err := reader.GetBlock(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			want := pack == mostlyLive || pack == afterFence || (pack == mostlyDead && !dead[name])
			if found != want || (found && string(data) != name) {
				t.Errorf("block %s = %q, %v; want found %v", name, data, found, want)
			}
		}
	}
}

// testPack is one packfile a test wrote.
type testPack struct {
	// id is the packfile id.
	id string
	// names are the block contents, in write order.
	names []string
	// refs are the block refs, in write order.
	refs []*block.BlockRef
}

// putPack writes one packfile holding a block of each name.
func putPack(t *testing.T, bucket *fakeBucket, store *PackStore, names ...string) *testPack {
	// Build a block of each name.
	t.Helper()
	pack := &testPack{names: names}
	batch := make([]*block.PutBatchEntry, len(names))
	for i, name := range names {
		ref, err := block.BuildBlockRef([]byte(name), nil)
		if err != nil {
			t.Fatal(err)
		}
		pack.refs = append(pack.refs, ref)
		batch[i] = &block.PutBatchEntry{Ref: ref, Data: []byte(name)}
	}

	// Write them and find the new packfile.
	before := bucket.keys()
	if err := store.PutBlockBatch(t.Context(), batch); err != nil {
		t.Fatal(err)
	}
	for _, key := range bucket.keys() {
		if id, ok := strings.CutPrefix(key, "p/"+packDir); ok && !slices.Contains(before, key) {
			pack.id = id
		}
	}
	return pack
}
