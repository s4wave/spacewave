package block_gc

import (
	"maps"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestResolveDeathsSharedPage applies two records to a page X that two Worlds
// share. World A replaces X's child C, which writes X again: X loses a count
// and lives, so C lives, and X's other child D gains the new X's edge. World
// B then drops X: X and C free, and D loses B's X and lives.
func TestResolveDeathsSharedPage(t *testing.T) {
	// Store A at rootA -> X -> {C, D}, and B at rootB -> X, with X counted.
	ctx := t.Context()
	rg := newTestRefGraph(t)
	page := func(name string) *block.BlockRef {
		t.Helper()
		ref, err := block.BuildBlockRef([]byte(name), nil)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	rootA, rootB, x, c, d := page("root a"), page("root b"), page("x"), page("c"), page("d")
	rootA2, x2, c2 := page("root a 2"), page("x 2"), page("c 2")
	edges := [][2]*block.BlockRef{
		{rootA, x}, {rootB, x}, {x, c}, {x, d},
		{rootA2, x2}, {x2, c2}, {x2, d},
	}
	for _, e := range edges {
		if err := rg.AddBlockRef(ctx, e[0], e[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := rg.AddHolds(ctx, []string{BlockIRI(x)}); err != nil {
		t.Fatal(err)
	}

	// apply resolves a record, adds its changes, and returns each page's fate.
	apply := func(from *block.BlockRef, dead *block.Replacements) map[string]bool {
		// Record each page's fate once.
		t.Helper()
		fates := make(map[string]bool)
		changes, err := rg.ResolveDeaths(ctx, from, dead, func(node string, frees bool) error {
			if _, ok := fates[node]; ok {
				t.Fatalf("%s visited twice", node)
			}
			fates[node] = frees
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if n, err := rg.ChangeHolds(ctx, changes); err != nil || n != 0 {
			t.Fatalf("underflows=%d err=%v", n, err)
		}
		return fates
	}

	// check compares the fates and the stored counts with the wanted ones.
	check := func(fates map[string]bool, want map[*block.BlockRef]bool, holds map[*block.BlockRef]uint64) {
		// Compare the fates.
		t.Helper()
		got := make(map[string]bool)
		for ref, frees := range want {
			got[BlockIRI(ref)] = frees
		}
		if !maps.Equal(fates, got) {
			t.Fatalf("fates %v, want %v", fates, got)
		}
		for ref, want := range holds {
			if n, err := rg.GetHolds(ctx, BlockIRI(ref)); err != nil || n != want {
				t.Fatalf("%s holds=%d err=%v, want %d", BlockIRI(ref), n, err, want)
			}
		}
	}

	// A replaces C: X loses its count and lives, C lives, D gains one.
	fates := apply(rootA, &block.Replacements{Replaced: []*block.BlockRef{rootA, x, c}})
	check(fates, map[*block.BlockRef]bool{rootA: true, x: false, c: false}, map[*block.BlockRef]uint64{x: 0, c: 0, d: 1})

	// B drops X: X and C free, and D loses B's X.
	fates = apply(rootB, &block.Replacements{Replaced: []*block.BlockRef{rootB}, Removed: []*block.BlockRef{x}})
	check(fates, map[*block.BlockRef]bool{rootB: true, x: true, c: true, d: false}, map[*block.BlockRef]uint64{d: 0})
}
