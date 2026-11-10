package packsweep

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// rewalkGraph is a tiny graph whose blocks are named by letter.
type rewalkGraph map[string]*block.BlockRef

// ref returns the named block's ref, building it on first use.
func (g rewalkGraph) ref(t *testing.T, name string) *block.BlockRef {
	t.Helper()
	if ref, ok := g[name]; ok {
		return ref
	}
	ref, err := block.BuildBlockRef([]byte("rewalk block "+name), nil)
	if err != nil {
		t.Fatal(err)
	}
	g[name] = ref
	return ref
}

// catalogue writes one pack holding each named parent with its children, as
// a snapshot reading a new pack does, and marks the snapshot complete.
func (g rewalkGraph) catalogue(t *testing.T, store kvtx.Store, id string, rows map[string][]string) {
	t.Helper()
	ctx := context.Background()
	rp := &readPack{id: id}
	for parent, children := range rows {
		refs := make([]*block.BlockRef, len(children))
		for i, c := range children {
			refs[i] = g.ref(t, c)
		}
		row, err := marshalRefs(refsKnown, refs)
		if err != nil {
			t.Fatal(err)
		}
		rp.keys = append(rp.keys, packKey{tail: RefTail(g.ref(t, parent)), state: refsKnown, refs: row})
	}
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := writeCatalogPack(ctx, tx, rp); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := updateHead(ctx, store, func(h *Head) error {
		h.Catalogued = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// names returns the holes' block names, sorted, each with its reason.
func (g rewalkGraph) names(holes []Hole) []string {
	var out []string
	for _, h := range holes {
		for name, ref := range g {
			if ref.EqualsRef(h.Ref) {
				out = append(out, name+":"+h.Reason)
			}
		}
	}
	slices.Sort(out)
	return out
}

// TestRewalkFromHoles walks a graph with two missing blocks, catalogues one
// of them in a later cycle, and re-walks from the old holes: the filled hole's
// missing children are the new holes, and the hole still missing stays one.
func TestRewalkFromHoles(t *testing.T) {
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	g := rewalkGraph{}

	// Walk the first cycle: C and D are missing.
	g.catalogue(t, store, "p1", map[string][]string{
		"root": {"A", "B"},
		"A":    {"C", "D"},
		"B":    {"E"},
		"E":    nil,
	})
	if _, err := Walk(ctx, &WalkOptions{Store: store, Roots: []*block.BlockRef{g.ref(t, "root")}}); err != nil {
		t.Fatal(err)
	}
	var old []*block.BlockRef
	var oldHoles []Hole
	if err := Holes(ctx, store, func(h Hole) error {
		old = append(old, h.Ref)
		oldHoles = append(oldHoles, h)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := g.names(oldHoles), []string{"C:missing", "D:missing"}; !slices.Equal(got, want) {
		t.Fatalf("first walk holes %v, want %v", got, want)
	}

	// Start a new cycle and catalogue a pack holding C, which reaches E,
	// already catalogued, and F and G, which no pack holds.
	if _, err := NewCycle(ctx, store); err != nil {
		t.Fatal(err)
	}
	g.catalogue(t, store, "p2", map[string][]string{"C": {"E", "F", "G"}})

	// Re-walk from the old holes, one repeated, a key a batch.
	res, err := Rewalk(ctx, &RewalkOptions{Store: store, Holes: append(old, old[0]), Batch: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := g.names(res.Holes), []string{"D:missing", "F:missing", "G:missing"}; !slices.Equal(got, want) {
		t.Fatalf("re-walk holes %v, want %v", got, want)
	}
	if res.Filled != 1 || res.Marked != 5 {
		t.Fatalf("filled %d marked %d, want 1 and 5", res.Filled, res.Marked)
	}
	if c := res.Counts(); c[packcopy.ReasonMissing] != 3 || len(c) != 1 {
		t.Fatalf("counts %v", c)
	}
	for i := 1; i < len(res.Holes); i++ {
		if string(RefTail(res.Holes[i-1].Ref)) >= string(RefTail(res.Holes[i].Ref)) {
			t.Fatal("holes not in tail order")
		}
	}

	// The re-walk wrote nothing: the new cycle has no walk state.
	for _, prefix := range []string{visitedPrefix, frontierPrefix, holePrefix} {
		if n := countRows(t, store, prefix); n != 0 {
			t.Fatalf("re-walk wrote %d rows under %s", n, prefix)
		}
	}
}

// TestRewalkStillMissing re-walks from a hole no new pack filled, which a ref
// source cannot serve either: it stays the only hole.
func TestRewalkStillMissing(t *testing.T) {
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	g := rewalkGraph{}
	g.catalogue(t, store, "p1", map[string][]string{"root": {"D"}})

	res, err := Rewalk(ctx, &RewalkOptions{Store: store, Holes: []*block.BlockRef{g.ref(t, "D")}, Refs: SourceRefs{}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := g.names(res.Holes), []string{"D:missing"}; !slices.Equal(got, want) {
		t.Fatalf("re-walk holes %v, want %v", got, want)
	}
	if res.Filled != 0 || res.Marked != 1 {
		t.Fatalf("filled %d marked %d, want 0 and 1", res.Filled, res.Marked)
	}
}

// TestRewalkNeedsSnapshot refuses a re-walk before the snapshot completes.
func TestRewalkNeedsSnapshot(t *testing.T) {
	store := store_kvtx_inmem.NewStore()
	if _, err := Rewalk(context.Background(), &RewalkOptions{Store: store}); err == nil {
		t.Fatal("re-walk ran without a complete snapshot")
	}
}
