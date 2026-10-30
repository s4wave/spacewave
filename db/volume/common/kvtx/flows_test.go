package kvtx

import (
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
	logrus_test "github.com/sirupsen/logrus/hooks/test"
)

// TestFlowsCountNewBlocksByPath proves each write path counts the blocks it
// stored that the volume did not hold, and those stored without their
// references, so the flows name the caller that grows the storage. The first
// put without references on each path logs its caller once.
func TestFlowsCountNewBlocksByPath(t *testing.T) {
	// A direct write counts once; writing the same block again does not. A
	// leaf whose writer declares its references counts as known.
	v, _ := newPublicationTestVolume(t)
	logger, hook := logrus_test.NewNullLogger()
	v.SetLogger(logrus.NewEntry(logger))
	ctx := t.Context()
	direct := []byte("direct block")
	for range 2 {
		if _, _, err := v.PutBlock(ctx, direct, nil); err != nil {
			t.Fatal(err)
		}
	}
	leaf := []byte("direct leaf")
	if _, _, err := v.PutBlock(ctx, leaf, &block.PutOpts{RefsKnown: true}); err != nil {
		t.Fatal(err)
	}

	// An owned write and a cache fill count under their own paths, and a
	// repeated fill does not count.
	owned := []byte("owned block")
	if _, _, err := v.PrepareOwnedBlock(ctx, "bucket", owned, nil); err != nil {
		t.Fatal(err)
	}
	fill := []byte("filled block")
	for range 2 {
		if _, _, err := v.PrepareOwnedBlock(ctx, "bucket", fill, &block.PutOpts{CacheFill: true}); err != nil {
			t.Fatal(err)
		}
	}

	// A stage writes a child and a root.
	_, release, _, _ := stageBuild(t, v, "flows")
	defer release()
	staged := len("flows child") + len("flows root")

	// A bucket's roots carry their entry.
	rootData := []byte("root entry")
	rootRef, err := block.BuildBlockRef(rootData, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := []*block.PutBatchEntry{{Ref: rootRef, Data: rootData}}
	if err := v.SetBucketRoots(ctx, "bucket", entries, []block.NamedRoot{{Name: "head", Ref: rootRef}}); err != nil {
		t.Fatal(err)
	}

	// A committed publication counts its entry.
	p := publicationFor(t, "store", "", "one")
	if err := awaitPublication(t, submitPublication(t, v, p)); err != nil {
		t.Fatal(err)
	}

	// Each path reports exactly its own blocks, all but the leaf without
	// their references.
	want := []volume.PutFlow{
		{Path: "direct", Blocks: 2, Bytes: uint64(len(direct) + len(leaf)), Unknown: 1},
		{Path: "publication", Blocks: 1, Bytes: uint64(len(p.Entries[0].Data)), Unknown: 1},
		{Path: "owned", Blocks: 1, Bytes: uint64(len(owned)), Unknown: 1},
		{Path: "fill", Blocks: 1, Bytes: uint64(len(fill)), Unknown: 1},
		{Path: "staged", Blocks: 2, Bytes: uint64(staged), Unknown: 2},
		{Path: "roots", Blocks: 1, Bytes: uint64(len(rootData)), Unknown: 1},
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
