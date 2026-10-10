package block_store_s3

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	if _, err := store.Reclaim(ctx, fence, live); err != nil {
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

// TestPackStoreReclaimSchedule runs the first pass at once, skips the fence
// when no packfile is worth reclaiming, and skips later passes until the
// blocks expected to die cost more to store than a pass costs, including in a
// new store that reads the recorded state. A store no block died in expects
// no further pass until it is written again.
func TestPackStoreReclaimSchedule(t *testing.T) {
	// Write a live packfile to a bucket priced as Amazon S3.
	ctx := t.Context()
	bucket, client := newFakeBucket(t, nil)
	store := newTestPackStore(t, client)
	store.pricing = awsPricing
	putPack(t, bucket, store, "a0", "a1")

	// Count the liveness checks and fail on any fence.
	var checks atomic.Int32
	live := func(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
		checks.Add(1)
		alive := make([]bool, len(refs))
		for i := range alive {
			alive[i] = true
		}
		return alive, nil
	}
	fence := func(context.Context) error {
		t.Error("fence ran with no packfile worth reclaiming")
		return nil
	}

	// Run the first pass and check it recorded one state and expects no
	// other.
	next, err := store.Reclaim(ctx, fence, live)
	if err != nil {
		t.Fatal(err)
	}
	if checks.Load() != 1 {
		t.Fatalf("first pass checked %d packfiles, want 1", checks.Load())
	}
	if !next.IsZero() {
		t.Fatalf("first pass expects another at %v", next)
	}
	states := reclaimStateKeys(bucket)
	if len(states) != 1 {
		t.Fatalf("bucket holds reclaim states %v", states)
	}

	// Check no block died, so neither this store nor a new one runs a pass.
	reader := newTestPackStore(t, client)
	reader.pricing = awsPricing
	for _, s := range []*PackStore{store, reader} {
		next, err := s.Reclaim(ctx, fence, live)
		if err != nil {
			t.Fatal(err)
		}
		if !next.IsZero() {
			t.Fatalf("skipped pass expects another at %v", next)
		}
	}
	if checks.Load() != 1 {
		t.Fatalf("skipped passes checked %d packfiles", checks.Load()-1)
	}

	// Check a write schedules a pass reclaimMaxInterval after the last.
	putPack(t, bucket, store, "b0")
	next, err = store.Reclaim(ctx, fence, live)
	if err != nil {
		t.Fatal(err)
	}
	if until := time.Until(next); until < reclaimMaxInterval-time.Hour || until > reclaimMaxInterval {
		t.Fatalf("written store expects a pass in %v", until)
	}

	// Check a self-hosted bucket, where a pass costs nothing, runs it and
	// replaces the recorded state.
	selfHosted := newTestPackStore(t, client)
	if _, err := selfHosted.Reclaim(ctx, fence, live); err != nil {
		t.Fatal(err)
	}
	if checks.Load() != 3 {
		t.Fatalf("self-hosted pass checked %d packfiles, want 2", checks.Load()-1)
	}
	if next := reclaimStateKeys(bucket); len(next) != 1 || next[0] == states[0] {
		t.Fatalf("bucket holds reclaim states %v after %v", next, states)
	}
}

// TestPackStoreReclaimFenceFailure records a pass whose fence failed, so the
// next request does not scan the bucket again before a pass is due, and
// returns the time it comes due.
func TestPackStoreReclaimFenceFailure(t *testing.T) {
	// Write a dead 1 MiB block to a bucket priced as Amazon S3.
	ctx := t.Context()
	bucket, client := newFakeBucket(t, nil)
	store := newTestPackStore(t, client)
	store.pricing = awsPricing
	putPack(t, bucket, store, strings.Repeat("x", 1<<20))

	// Count the liveness checks, report every block dead, and fail the fence.
	var checks atomic.Int32
	live := func(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
		checks.Add(1)
		return make([]bool, len(refs)), nil
	}
	errNotReady := errors.New("not ready")
	fence := func(context.Context) error { return errNotReady }

	// Run a pass and check it returns the fence error and a due time.
	next, err := store.Reclaim(ctx, fence, live)
	if !errors.Is(err, errNotReady) {
		t.Fatalf("Reclaim = %v, want the fence error", err)
	}
	if len(reclaimStateKeys(bucket)) != 1 {
		t.Fatal("failed pass was not recorded")
	}
	if until := time.Until(next); until <= 0 || until >= reclaimMaxInterval {
		t.Fatalf("failed pass expects the next in %v", until)
	}

	// Check the next request skips the scan and keeps the due time.
	skipped, err := store.Reclaim(ctx, fence, live)
	if err != nil {
		t.Fatal(err)
	}
	if checks.Load() != 1 {
		t.Fatalf("passes checked %d packfiles, want 1", checks.Load())
	}
	if !skipped.Equal(next) {
		t.Fatalf("skipped pass expects the next at %v, want %v", skipped, next)
	}
}

// reclaimStateKeys returns the keys of the reclaim states in the bucket.
func reclaimStateKeys(bucket *fakeBucket) []string {
	return slices.DeleteFunc(bucket.keys(), func(key string) bool {
		return !strings.HasPrefix(key, "p/"+reclaimDir)
	})
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
	if _, err := store.PutBlockBatch(t.Context(), batch); err != nil {
		t.Fatal(err)
	}
	for _, key := range bucket.keys() {
		if id, ok := strings.CutPrefix(key, "p/"+packDir); ok && !slices.Contains(before, key) {
			pack.id = id
		}
	}
	return pack
}
