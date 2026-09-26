package world_block_test

import (
	"fmt"
	"iter"
	"testing"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// exampleObjects yields an example body named after each key, in the given
// order.
func exampleObjects(keys ...string) iter.Seq2[string, block.Block] {
	return func(yield func(string, block.Block) bool) {
		for _, key := range keys {
			if !yield(key, block_mock.NewExample(key)) {
				return
			}
		}
	}
}

// accessImportedWorld opens an imported World for reading.
func accessImportedWorld(t *testing.T, tb *world_testbed.Testbed, ref *bucket.ObjectRef, cb func(*world_block.WorldState)) {
	t.Helper()
	ctx := t.Context()
	err := tb.Engine.AccessWorldState(ctx, ref, func(cursor *bucket_lookup.Cursor) error {
		state, err := world_block.BuildWorldStateFromCursor(ctx, tb.Logger, false, cursor, tb.Engine, nil, false)
		if err != nil {
			return err
		}
		defer state.Discard()
		cb(state)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestImportSnapshotPreservesWorld verifies packed readback of bodies,
// revisions from relationship ends, relationships, and the World sequence. The
// import holds more blocks than the write buffer, so it drains before its root
// exists.
func TestImportSnapshotPreservesWorld(t *testing.T) {
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	keys := make([]string, 4200)
	for i := range keys {
		keys[i] = fmt.Sprintf("object-%04d", i)
	}
	quads := []world.GraphQuad{
		world.NewGraphQuadWithKeys("object-0000", "<edge>", "object-0001", ""),
		world.NewGraphQuadWithKeys("object-0000", "<edge>", "object-0000", ""),
	}
	ref, err := world_block.ImportSnapshot(ctx, tb.Engine, exampleObjects(keys...), quads)
	if err != nil {
		t.Fatal(err)
	}

	accessImportedWorld(t, tb, ref, func(state *world_block.WorldState) {
		seq, err := state.GetSeqno(ctx)
		if err != nil || seq != uint64(len(keys)+len(quads)) {
			t.Fatalf("sequence %d, error %v", seq, err)
		}
		refs, err := state.GetObjectRootRefsBatch(ctx, []string{"object-0000", "object-0001", "object-4199", "object-4200"})
		if err != nil {
			t.Fatal(err)
		}
		if refs[0].Rev != 4 || refs[1].Rev != 2 || refs[2].Rev != 1 || refs[3].Exists {
			t.Fatalf("unexpected object revisions: %v", refs)
		}
		body, err := world.LookupObjectBody[*block_mock.Example](ctx, state, "object-4198", block_mock.NewExampleBlock)
		if err != nil || body.GetMsg() != "object-4198" {
			t.Fatalf("body %v, error %v", body, err)
		}
		found, err := state.LookupGraphQuads(ctx, world.NewGraphQuad("", "", "", ""), 0)
		if err != nil || len(found) != len(quads) {
			t.Fatalf("relationships %d, error %v", len(found), err)
		}
	})
}

// TestImportSnapshotGraphSpansBatches verifies that relationships spanning
// more than one graph import batch reuse nodes across batches and are all kept.
func TestImportSnapshotGraphSpansBatches(t *testing.T) {
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	quads := make([]world.GraphQuad, 8193)
	for i := range quads {
		quads[i] = world.NewGraphQuadWithKeys("from", "<edge>", "to", fmt.Sprintf("<label-%05d>", i))
	}
	ref, err := world_block.ImportSnapshot(ctx, tb.Engine, exampleObjects("from", "to"), quads)
	if err != nil {
		t.Fatal(err)
	}

	accessImportedWorld(t, tb, ref, func(state *world_block.WorldState) {
		found, err := state.LookupGraphQuads(ctx, world.NewGraphQuad("", "", "", ""), 0)
		if err != nil || len(found) != len(quads) {
			t.Fatalf("graph contains %d relationships, want %d: %v", len(found), len(quads), err)
		}
		for _, q := range []world.GraphQuad{quads[0], quads[len(quads)-1]} {
			found, err := state.LookupGraphQuads(ctx, q, 1)
			if err != nil || len(found) != 1 {
				t.Fatalf("missing relationship %v: %v", q, err)
			}
		}
	})
}

// TestImportSnapshotRejectsInvalidContents returns no publishable reference
// for unsorted objects, a missing endpoint, or a repeated relationship.
func TestImportSnapshotRejectsInvalidContents(t *testing.T) {
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	edge := world.NewGraphQuadWithKeys("a", "<edge>", "b", "")
	for _, tc := range []struct {
		name  string
		keys  []string
		quads []world.GraphQuad
		match func(error) bool
	}{
		{name: "unsorted objects", keys: []string{"b", "a"}, match: func(err error) bool {
			return errors.Is(err, kvtx_block_okra.ErrUnsortedEntries)
		}},
		{name: "missing endpoint", keys: []string{"a"}, quads: []world.GraphQuad{edge}, match: func(err error) bool {
			return errors.Is(err, world.ErrObjectNotFound)
		}},
		{name: "repeated relationship", keys: []string{"a", "b"}, quads: []world.GraphQuad{edge, edge}, match: graph.IsQuadExist},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := world_block.ImportSnapshot(ctx, tb.Engine, exampleObjects(tc.keys...), tc.quads)
			if ref != nil || !tc.match(err) {
				t.Fatalf("import returned ref %v, error %v", ref, err)
			}
		})
	}
}
