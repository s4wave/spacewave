package kvtx

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
	logrus_test "github.com/sirupsen/logrus/hooks/test"
)

// TestFlowsCountNewBlocksByPath proves each write path counts the blocks it
// stored that the volume did not hold, so the flows name the caller that grows
// the storage, and its puts of blocks the volume held. On a settled Volume each
// path refuses a new block written without its references, and a fill drops
// one; either stores nothing for it and counts it, and the path logs its first
// caller once.
func TestFlowsCountNewBlocksByPath(t *testing.T) {
	// A direct write counts once, and writing the same block again counts as
	// existing.
	v, _ := newTestVolume(t)
	logger, hook := logrus_test.NewNullLogger()
	v.SetLogger(logrus.NewEntry(logger))
	ctx := t.Context()
	known := &block.PutOpts{RefsKnown: true}
	direct := []byte("direct block")
	for range 2 {
		if _, _, err := v.PutBlock(ctx, direct, known); err != nil {
			t.Fatal(err)
		}
	}

	// Writing the stored block again without its child list holds it again
	// and keeps its list.
	if _, _, err := v.PutBlock(ctx, direct, nil); err != nil {
		t.Fatalf("stored block put without references = %v; want it held again", err)
	}

	// An owned write and a cache fill count under their own paths, and a
	// repeated fill counts as existing.
	owned := []byte("owned block")
	if _, _, err := v.PrepareOwnedBlock(ctx, "bucket", owned, known); err != nil {
		t.Fatal(err)
	}
	fill := []byte("filled block")
	for range 2 {
		if _, _, err := v.PrepareOwnedBlock(ctx, "bucket", fill, &block.PutOpts{CacheFill: true, RefsKnown: true}); err != nil {
			t.Fatal(err)
		}
	}

	// A stage writes a child and a root.
	stage, release, err := v.OpenStage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	child, _, err := v.PrepareStagedBlock(ctx, stage, []byte("flows child"), known)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := v.PrepareStagedBlock(ctx, stage, []byte("flows root"), &block.PutOpts{Refs: []*block.BlockRef{child}, RefsKnown: true}); err != nil {
		t.Fatal(err)
	}
	staged := len("flows child") + len("flows root")

	// A bucket's roots carry their entry.
	rootData := []byte("root entry")
	rootRef, err := block.BuildBlockRef(rootData, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := []*block.PutBatchEntry{{Ref: rootRef, Data: rootData, RefsKnown: true}}
	if err := v.SetBucketRoots(ctx, "bucket", entries, []block.NamedRoot{{Name: "head", Ref: rootRef}}); err != nil {
		t.Fatal(err)
	}

	// A committed batch counts its entry.
	batch := batchFor(t, "store", "one")
	if err := putBatch(ctx, v, batch); err != nil {
		t.Fatal(err)
	}

	// Each path but the fill refuses a block without its child list, and the
	// fill drops it.
	unknown := func(path string) []byte { return []byte("unknown " + path) }
	unknownEntry := func(path string) *block.PutBatchEntry {
		ref, err := block.BuildBlockRef(unknown(path), nil)
		if err != nil {
			t.Fatal(err)
		}
		return &block.PutBatchEntry{Ref: ref, Data: unknown(path)}
	}
	refused := unknownEntry("batch")
	roots := unknownEntry("roots")
	puts := map[string]func() error{
		"direct": func() error {
			_, _, err := v.PutBlock(ctx, unknown("direct"), nil)
			return err
		},
		"batch": func() error {
			return putBatch(ctx, v, []*block.PutBatchEntry{refused})
		},
		"owned": func() error {
			_, _, err := v.PrepareOwnedBlock(ctx, "bucket", unknown("owned"), nil)
			return err
		},
		"staged": func() error {
			_, _, err := v.PrepareStagedBlock(ctx, stage, unknown("staged"), nil)
			return err
		},
		"roots": func() error {
			return v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{roots}, []block.NamedRoot{{Name: "head", Ref: roots.Ref, Previous: rootRef}})
		},
	}
	for path, put := range puts {
		if err := put(); !errors.Is(err, block.ErrRefsUnknown) {
			t.Fatalf("path %s put without references = %v; want refused", path, err)
		}
		if found, err := v.GetBlockExists(ctx, unknownEntry(path).Ref); err != nil || found {
			t.Fatalf("path %s stored its refused block: %v %v", path, found, err)
		}
	}
	if _, _, err := v.PrepareOwnedBlock(ctx, "bucket", unknown("fill"), &block.PutOpts{CacheFill: true}); err != nil {
		t.Fatalf("fill without references = %v; want it dropped", err)
	}
	if found, err := v.GetBlockExists(ctx, unknownEntry("fill").Ref); err != nil || found {
		t.Fatalf("the fill stored its dropped block: %v %v", found, err)
	}

	// Each path reports exactly its own blocks, its repeated puts and one
	// refusal, or the fill's one drop.
	want := []volume.PutFlow{
		{Path: "direct", Blocks: 2, Bytes: uint64(len(direct) + len(batch[0].Data)), Existing: 2, Refused: 2},
		{Path: "owned", Blocks: 1, Bytes: uint64(len(owned)), Refused: 1},
		{Path: "fill", Blocks: 1, Bytes: uint64(len(fill)), Existing: 1, Dropped: 1},
		{Path: "staged", Blocks: 2, Bytes: uint64(staged), Refused: 1},
		{Path: "roots", Blocks: 1, Bytes: uint64(len(rootData)), Refused: 1},
	}
	flows := v.GetFlows()
	if flows.VolumeID != v.GetID() || len(flows.Puts) != len(want) {
		t.Fatalf("flows = %+v; want volume %s with %d paths", flows, v.GetID(), len(want))
	}
	for i, w := range want {
		if flows.Puts[i] != w {
			t.Fatalf("path %d = %+v; want %+v", i, flows.Puts[i], w)
		}
	}

	// Each path logged its first writer once, from the writer's goroutine.
	logged := make(map[string]string)
	for _, e := range hook.AllEntries() {
		path, _ := e.Data["path"].(string)
		caller, _ := e.Data["caller"].(string)
		if _, dup := logged[path]; dup {
			t.Fatalf("path %s logged twice", path)
		}
		logged[path] = caller
	}
	for _, w := range want {
		if !strings.Contains(logged[w.Path], "_test.go") {
			t.Fatalf("path %s caller = %q; want the test's stack", w.Path, logged[w.Path])
		}
	}
}

// TestSetBucketRootsRefusesDuplicateName moves 3 and 30,000 roots in one
// change, with one name repeated at the first, middle or last position. Each
// change is refused whole with the duplicate's error and lands no root; the
// same roots without the duplicate all land.
func TestSetBucketRootsRefusesDuplicateName(t *testing.T) {
	// Every root holds one leaf block, carried by each change.
	ctx := t.Context()
	v, _ := newTestVolume(t)
	data := []byte("duplicate root leaf")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}
	roots := func(prefix string, n int) []block.NamedRoot {
		out := make([]block.NamedRoot, n)
		for i := range out {
			out[i] = block.NamedRoot{Name: prefix + strconv.Itoa(i), Ref: ref}
		}
		return out
	}
	count := func(prefix string) int {
		var n int
		if err := v.ListBucketRoots(ctx, "bucket", prefix, func(block.NamedRoot) error {
			n++
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}

	for _, n := range []int{3, 30000} {
		for _, pos := range []int{1, n / 2, n - 1} {
			// Repeat the first root's name at pos; the change is refused.
			prefix := "dup-" + strconv.Itoa(n) + "-" + strconv.Itoa(pos) + "/"
			change := roots(prefix, n)
			change[pos].Name = change[0].Name
			err := v.SetBucketRoots(ctx, "bucket", entries, change)
			want := "root \"" + change[0].Name + "\" moves twice in one change"
			if err == nil || err.Error() != want {
				t.Fatalf("%d roots, duplicate at %d: got %v, want %q", n, pos, err, want)
			}
			if got := count(prefix); got != 0 {
				t.Fatalf("%d roots, duplicate at %d: a refused change landed %d roots", n, pos, got)
			}
		}

		// Without the duplicate every root lands.
		prefix := "ok-" + strconv.Itoa(n) + "/"
		if err := v.SetBucketRoots(ctx, "bucket", entries, roots(prefix, n)); err != nil {
			t.Fatalf("%d roots: %v", n, err)
		}
		if got := count(prefix); got != n {
			t.Fatalf("%d roots: %d landed", n, got)
		}
	}
}
