package kvtx

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
)

// TestFlowsCountNewBlocksByPath proves each write path counts the blocks it
// stored that the volume did not hold, so the flows name the caller that
// grows the storage.
func TestFlowsCountNewBlocksByPath(t *testing.T) {
	// A direct write counts once; writing the same block again does not.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	direct := []byte("direct block")
	for range 2 {
		if _, _, err := v.PutBlock(ctx, direct, nil); err != nil {
			t.Fatal(err)
		}
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

	// Each path reports exactly its own blocks, none with declared references.
	want := []volume.PutFlow{
		{Path: "direct", Blocks: 1, Bytes: uint64(len(direct)), RefsUnknown: 1},
		{Path: "publication", Blocks: 1, Bytes: uint64(len(p.Entries[0].Data)), RefsUnknown: 1},
		{Path: "owned", Blocks: 1, Bytes: uint64(len(owned)), RefsUnknown: 1},
		{Path: "fill", Blocks: 1, Bytes: uint64(len(fill)), RefsUnknown: 1},
		{Path: "staged", Blocks: 2, Bytes: uint64(staged), RefsUnknown: 2},
		{Path: "roots", Blocks: 1, Bytes: uint64(len(rootData)), RefsUnknown: 1},
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
}

// TestFlowsNameCallersOfPutsWithoutRefs proves the flows count only the puts
// whose writer did not declare references, and name the code that made them.
func TestFlowsNameCallersOfPutsWithoutRefs(t *testing.T) {
	// Put one block with declared references, and two without, one of them
	// twice.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	if _, _, err := v.PutBlock(ctx, []byte("known"), &block.PutOpts{RefsKnown: true}); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"unknown one", "unknown two", "unknown two"} {
		if _, _, err := v.PutBlock(ctx, []byte(data), nil); err != nil {
			t.Fatal(err)
		}
	}

	// The direct path stored three blocks, two without references.
	flows := v.GetFlows()
	if got := flows.Puts[putDirect]; got.Blocks != 3 || got.RefsUnknown != 2 {
		t.Fatalf("direct flow = %+v; want 3 blocks, 2 without references", got)
	}

	// One caller made all three puts without references. The volume's own
	// frames, this test's among them, are skipped, so the test runner is named.
	if len(flows.UnknownCallers) != 1 {
		t.Fatalf("callers = %+v; want one", flows.UnknownCallers)
	}
	c := flows.UnknownCallers[0]
	if c.Path != "direct" || c.Puts != 3 || c.Caller != "testing.tRunner" {
		t.Fatalf("caller = %+v; want the test runner's three direct puts", c)
	}
}
