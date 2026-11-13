package world_block_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// quadKey identifies a relationship for comparison.
func quadKey(q world.GraphQuad) string {
	return fmt.Sprintf("%s %s %s %s", q.GetSubject(), q.GetPredicate(), q.GetObj(), q.GetLabel())
}

// worldQuads returns every relationship of the World at ref, sorted.
func worldQuads(t *testing.T, tb *world_testbed.Testbed, ref *bucket.ObjectRef) []string {
	t.Helper()
	var out []string
	accessImportedWorld(t, tb, ref, func(state *world_block.WorldState) {
		found, err := state.LookupGraphQuads(t.Context(), world.NewGraphQuad("", "", "", ""), 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range found {
			out = append(out, quadKey(q))
		}
	})
	slices.Sort(out)
	return out
}

// TestBuildNodeGraphRootKeepsWorld checks that the build of a kv World, after
// deletes that free IDs, holds the same objects and relationships, writes the
// same root at any sort budget, and takes new writes.
func TestBuildNodeGraphRootKeepsWorld(t *testing.T) {
	// Import a kv World with shared predicates and some labels.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	keys := make([]string, 40)
	for i := range keys {
		keys[i] = fmt.Sprintf("object-%02d", i)
	}
	seen := make(map[string]bool)
	var quads []world.GraphQuad
	for i := range 200 {
		label := ""
		if i%5 == 0 {
			label = fmt.Sprintf("<label-%d>", i%3)
		}
		q := world.NewGraphQuadWithKeys(keys[i%40], fmt.Sprintf("<pred-%d>", i%4), keys[(i*7+i/40)%40], label)
		if !seen[quadKey(q)] {
			seen[quadKey(q)] = true
			quads = append(quads, q)
		}
	}
	base, err := world_block.ImportSnapshot(ctx, engineStage(t, tb.Engine), exampleObjects(keys...), quads)
	if err != nil {
		t.Fatal(err)
	}

	// Delete every ninth relationship, freeing IDs in the old log.
	base, err = world_block.UpdateSnapshot(ctx, tb.Logger, engineStage(t, tb.Engine), base, func(ctx context.Context, state *world_block.WorldState) error {
		for i := 0; i < len(quads); i += 9 {
			if err := state.DeleteGraphQuad(ctx, quads[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := worldQuads(t, tb, base)

	// Build at a budget that holds everything and at one that spills often.
	var roots []*block.BlockRef
	for _, budget := range []int{1 << 20, 512} {
		err := engineStage(t, tb.Engine).AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
			root, err := world_block.BuildNodeGraphRoot(ctx, cursor, nil, base.GetRootRef(), world_block.RegenConfig{SpillDir: t.TempDir(), Budget: budget})
			roots = append(roots, root)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !roots[0].EqualVT(roots[1]) {
		t.Fatal("the sort budget changed the built root")
	}
	built := base.CloneVT()
	built.RootRef = roots[0]

	// The built World holds the same relationships and objects.
	if got := worldQuads(t, tb, built); !slices.Equal(got, want) {
		t.Fatalf("built World holds %d relationships, want %d", len(got), len(want))
	}
	var before, after []*world.ObjectRootRef
	accessImportedWorld(t, tb, base, func(state *world_block.WorldState) {
		before, err = state.GetObjectRootRefsBatch(ctx, keys)
	})
	accessImportedWorld(t, tb, built, func(state *world_block.WorldState) {
		if after, err = state.GetObjectRootRefsBatch(ctx, keys); err == nil {
			var root *world_block.World
			root, err = state.GetRoot(ctx)
			if f := root.GetFormat(); err == nil && f != world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH {
				t.Fatalf("built World has format %v", f)
			}
			// The import's Okra object tree keeps its layout under the node
			// graph's Okra tag.
			if impl := root.GetObjectKeyValue().GetImplType(); err == nil && impl != kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA_NODE_GRAPH {
				t.Fatalf("built World's object tree is %v", impl)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(before, after, func(a, b *world.ObjectRootRef) bool {
		return a.ObjectKey == b.ObjectKey && a.Rev == b.Rev && a.Exists == b.Exists && a.RootRef.EqualVT(b.RootRef)
	}) {
		t.Fatal("built World's objects differ")
	}

	// The built World takes a relationship on a new predicate and a delete.
	added := world.NewGraphQuadWithKeys(keys[0], "<pred-new>", keys[1], "")
	built, err = world_block.UpdateSnapshot(ctx, tb.Logger, engineStage(t, tb.Engine), built, func(ctx context.Context, state *world_block.WorldState) error {
		if err := state.SetGraphQuad(ctx, added); err != nil {
			return err
		}
		return state.DeleteGraphQuad(ctx, quads[1])
	})
	if err != nil {
		t.Fatal(err)
	}
	want = slices.DeleteFunc(append(want, quadKey(added)), func(k string) bool { return k == quadKey(quads[1]) })
	slices.Sort(want)
	if got := worldQuads(t, tb, built); !slices.Equal(got, want) {
		t.Fatalf("after a write the built World holds %d relationships, want %d", len(got), len(want))
	}
}
