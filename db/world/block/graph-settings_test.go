package world_block

import (
	"context"
	"fmt"
	"slices"
	"testing"

	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_cayley "github.com/s4wave/spacewave/db/kvtx/cayley"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// predicateIndexes are the default indexes plus one serving lookups that pin
// only the predicate.
var predicateIndexes = append(slices.Clone(cayley_kv.DefaultQuadIndexes), cayley_kv.QuadIndex{
	Dirs: []quad.Direction{quad.Predicate, quad.Label, quad.Subject},
})

// TestGraphSettingsAtCreation creates a World with the predicate index and one
// with the default indexes, applies the same writes to both, and checks that
// the first serves its graph through the predicate index with the same
// results, records the layout in the graph store, and replays to the same
// root.
func TestGraphSettingsAtCreation(t *testing.T) {
	// Build the settings holding the predicate index.
	ctx := context.Background()
	settings, err := NewGraphSettings(predicateIndexes)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := settings.ValidateInitial(); err != nil {
		t.Fatal(err.Error())
	}

	// Build the Worlds: two members with the index, one without.
	memberA := newSettingsTestWorld(t, ctx, settings)
	memberB := newSettingsTestWorld(t, ctx, settings)
	plain := newSettingsTestWorld(t, ctx, nil)

	// Both members replay to the same root, which differs from the default
	// World's because it holds the index postings.
	if !memberA.GetRootRef().EqualVT(memberB.GetRootRef()) {
		t.Fatal("members with the same settings reached different roots")
	}
	if memberA.GetRootRef().EqualVT(plain.GetRootRef()) {
		t.Fatal("the predicate index changed nothing in the World")
	}

	// The open graph uses the root's layout.
	root, err := memberA.GetRoot(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !root.GetGraphSettings().EqualVT(settings) {
		t.Fatal("World root lost its graph settings")
	}
	graphHd, err := memberA.graph(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := graphHd.QuadStore.(*cayley_kv.QuadStore).Layout(); !cayley_kv.CompareQuadIndexes(got, predicateIndexes) {
		t.Fatalf("open graph layout %v", got)
	}

	// The graph store records the same layout, so a reader that trusts nothing
	// agrees with the root.
	if got := readStoredLayout(t, ctx, memberA); !cayley_kv.CompareQuadIndexes(got, predicateIndexes) {
		t.Fatalf("stored graph layout %v", got)
	}
	if got := readStoredLayout(t, ctx, plain); !cayley_kv.CompareQuadIndexes(got, cayley_kv.DefaultQuadIndexes) {
		t.Fatalf("default stored graph layout %v", got)
	}

	// Lookups answer the same with and without the index, in the same order.
	filters := []world.GraphQuad{
		world.NewGraphQuad("", "<settings/rel-1>", "", ""),
		world.NewGraphQuad("", "<settings/rel-2>", "", ""),
		world.NewGraphQuadWithKeys("settings/obj-3", "", "", ""),
		world.NewGraphQuadWithKeys("", "<settings/rel-0>", "settings/obj-0", ""),
	}
	for _, filter := range filters {
		want, err := plain.LookupGraphQuads(ctx, filter, 0)
		if err != nil {
			t.Fatal(err.Error())
		}
		got, err := memberA.LookupGraphQuads(ctx, filter, 0)
		if err != nil {
			t.Fatal(err.Error())
		}
		if len(want) == 0 || !slices.Equal(quadStrings(got), quadStrings(want)) {
			t.Fatalf("lookup %v: got %v, want %v", quadStrings([]world.GraphQuad{filter}), quadStrings(got), quadStrings(want))
		}
	}

	// The typed listing finds its reverse index in the extended layout.
	for _, ws := range []*WorldState{plain, memberA} {
		keys, more, err := ws.ListObjectsWithType(ctx, "settings/type", "", 100)
		if err != nil {
			t.Fatal(err.Error())
		}
		if more || len(keys) != 4 {
			t.Fatalf("typed listing %v more=%v", keys, more)
		}
	}
}

// newSettingsTestWorld creates a World starting with settings, applies a
// fixed set of writes, and commits them.
func newSettingsTestWorld(t *testing.T, ctx context.Context, settings *GraphSettings) *WorldState {
	// Build the testbed and its empty cursor.
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(ocs.Release)

	// Write the initial World root with the settings.
	btx, bcs := ocs.BuildTransaction(nil)
	root := NewWorld(false)
	root.GraphSettings = settings.CloneVT()
	bcs.SetBlock(root, true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(rootRef)

	// Open the World.
	ws, err := BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(ws.Discard)

	// Create the objects and link them under three predicates.
	for i := range 6 {
		createTestObject(t, ctx, ws, fmt.Sprintf("settings/obj-%d", i))
	}
	for i := range 6 {
		for j := range 6 {
			if i == j || (i*j)%4 == 1 {
				continue
			}
			q := world.NewGraphQuadWithKeys(
				fmt.Sprintf("settings/obj-%d", i),
				fmt.Sprintf("<settings/rel-%d>", i%3),
				fmt.Sprintf("settings/obj-%d", j),
				"",
			)
			if err := ws.SetGraphQuad(ctx, q); err != nil {
				t.Fatal(err.Error())
			}
		}
	}

	// Type four objects, delete one link, and commit.
	for i := range 4 {
		if err := world_types.SetObjectType(ctx, ws, fmt.Sprintf("settings/obj-%d", i), "settings/type"); err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := ws.DeleteGraphQuad(ctx, world.NewGraphQuadWithKeys("settings/obj-1", "<settings/rel-1>", "settings/obj-2", "")); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	return ws
}

// readStoredLayout opens the committed graph of ws without options, so cayley
// reads the layout the store records.
func readStoredLayout(t *testing.T, ctx context.Context, ws *WorldState) []cayley_kv.QuadIndex {
	// Open the graph tree read-only and read the layout cayley loads.
	t.Helper()
	ktx, err := kvtx_block.BuildKvTransaction(ctx, ws.GetBcs().FollowSubBlock(2), false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ktx.Discard()
	hd, err := kvtx_cayley.NewGraph(ctx, kvtx.NewTxStore(ktx), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer hd.Close()
	return hd.QuadStore.(*cayley_kv.QuadStore).Layout()
}

// quadStrings formats quads for comparison.
func quadStrings(quads []world.GraphQuad) []string {
	out := make([]string, len(quads))
	for i, q := range quads {
		out[i] = q.GetSubject() + " " + q.GetPredicate() + " " + q.GetObj() + " " + q.GetLabel()
	}
	return out
}
