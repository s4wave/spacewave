package kvtx

import (
	"errors"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
	logrus_test "github.com/sirupsen/logrus/hooks/test"
)

// TestFlowsCountNewBlocksByPath proves each write path counts the blocks it
// stored that the volume did not hold, so the flows name the caller that grows
// the storage. Each path refuses a new block written without its references,
// stores nothing for it, counts the refusal, and logs its first caller once.
func TestFlowsCountNewBlocksByPath(t *testing.T) {
	// A direct write counts once; writing the same block again does not, and
	// writing it again without its references passes, since it is stored.
	v, _ := newPublicationTestVolume(t)
	logger, hook := logrus_test.NewNullLogger()
	v.SetLogger(logrus.NewEntry(logger))
	ctx := t.Context()
	known := &block.PutOpts{RefsKnown: true}
	direct := []byte("direct block")
	for _, opts := range []*block.PutOpts{known, known, nil} {
		if _, _, err := v.PutBlock(ctx, direct, opts); err != nil {
			t.Fatal(err)
		}
	}

	// An owned write and a cache fill count under their own paths, and a
	// repeated fill does not count.
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
	stage, release, _, _ := stageBuild(t, v, "flows")
	defer release()
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

	// A committed publication counts its entry.
	p := publicationFor(t, "store", "", "one")
	if err := awaitPublication(t, submitPublication(t, v, p)); err != nil {
		t.Fatal(err)
	}

	// Each path refuses a new block without its references.
	unknown := func(path string) []byte { return []byte("unknown " + path) }
	unknownEntry := func(path string) *block.PutBatchEntry {
		ref, err := block.BuildBlockRef(unknown(path), nil)
		if err != nil {
			t.Fatal(err)
		}
		return &block.PutBatchEntry{Ref: ref, Data: unknown(path)}
	}
	refused := unknownEntry("publication")
	unknownPub := publicationFor(t, "store", "one", "two")
	unknownPub.Entries = []*block.PutBatchEntry{refused}
	roots := unknownEntry("roots")
	puts := map[string]func() error{
		"direct": func() error {
			_, _, err := v.PutBlock(ctx, unknown("direct"), nil)
			return err
		},
		"publication": func() error {
			return awaitPublication(t, submitPublication(t, v, unknownPub))
		},
		"owned": func() error {
			_, _, err := v.PrepareOwnedBlock(ctx, "bucket", unknown("owned"), nil)
			return err
		},
		"fill": func() error {
			_, _, err := v.PrepareOwnedBlock(ctx, "bucket", unknown("fill"), &block.PutOpts{CacheFill: true})
			return err
		},
		"staged": func() error {
			_, _, err := v.PrepareStagedBlock(ctx, stage, unknown("staged"), nil)
			return err
		},
		"roots": func() error {
			return v.SetBucketRoots(ctx, "bucket", []*block.PutBatchEntry{roots}, []block.NamedRoot{{Name: "head", Ref: roots.Ref}})
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

	// Each path reports exactly its own blocks and one refusal.
	want := []volume.PutFlow{
		{Path: "direct", Blocks: 1, Bytes: uint64(len(direct)), Refused: 1},
		{Path: "publication", Blocks: 1, Bytes: uint64(len(p.Entries[0].Data)), Refused: 1},
		{Path: "owned", Blocks: 1, Bytes: uint64(len(owned)), Refused: 1},
		{Path: "fill", Blocks: 1, Bytes: uint64(len(fill)), Refused: 1},
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
