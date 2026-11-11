package block_gc

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestNormalizeChildrenAllocs checks that normalizing a page's child list
// allocates its output once, at 32 and 128 children, rather than a block key
// per comparison inside the writer.
func TestNormalizeChildrenAllocs(t *testing.T) {
	for _, n := range []int{32, 128} {
		// Build n children and repeat the first.
		children := make([]*block.BlockRef, 0, n+1)
		for i := range n {
			ref, err := block.BuildBlockRef([]byte("child "+strconv.Itoa(i)), nil)
			if err != nil {
				t.Fatal(err)
			}
			children = append(children, ref)
		}
		children = append(children, children[0])

		// Normalize them and count the allocations.
		got := normalizeChildren(children)
		if len(got) != n {
			t.Fatalf("%d children normalized to %d", n, len(got))
		}
		if allocs := testing.AllocsPerRun(20, func() { normalizeChildren(children) }); allocs > 1 {
			t.Errorf("normalizing %d children made %v allocations, want 1", n, allocs)
		}
	}
}

// TestSettleWaitsForNetting checks that neither the front pop nor the flip
// runs while netting owes a write: a due down or up cascade or a paged
// release (the model's FlipSettled, mutant rps-noflipsettled), or an
// unapplied fill up (FlipFills, mutant adopt-revive-fill-noflipfills). Each
// blocker leaves a counted block at zero on a ripe sequence past nothing, so
// the free proof would pop it and the flip would follow, unless the check
// holds. Collect nets before the settle runs, so no writer reaches these
// states today; the test writes them as a netting step cut short would.
func TestSettleWaitsForNetting(t *testing.T) {
	ctx := context.Background()
	ref, err := block.BuildBlockRef([]byte("front block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	key := BlockKey(ref)
	blockers := []struct {
		name  string
		block func(rec *Records, state *LivenessState) error
	}{
		{"a down cascade", func(rec *Records, state *LivenessState) error {
			return rec.addCascade(ctx, state, key, cascadeDown)
		}},
		{"an up cascade", func(rec *Records, state *LivenessState) error {
			return rec.addCascade(ctx, state, key, cascadeUp)
		}},
		{"a paged release", func(_ *Records, state *LivenessState) error {
			state.Release = &Release{Block: key}
			return nil
		}},
		{"an unapplied fill up", func(rec *Records, _ *LivenessState) error {
			return rec.tx.Set(ctx, rec.key(kindFillUps, key), []byte{0})
		}},
		{"nothing", nil},
	}
	for _, bc := range blockers {
		t.Run(bc.name, func(t *testing.T) {
			// An adopted Open Volume with the run complete and one listed,
			// unheld, ripe block on the front.
			tx, err := store_kvtx_inmem.NewStore().NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()
			rec := NewRecords(tx, []byte("l/"))
			state := &LivenessState{
				Adoption:   Adoption_ADOPTION_ADOPTED,
				Generation: Generation_GENERATION_OPEN,
				Cursor:     10,
			}
			for _, err := range []error{
				rec.PutChildren(ctx, ref, nil),
				rec.SetHolders(ctx, ref, 0, 5),
				tx.Set(ctx, rec.key(kindFront, key), nil),
				tx.Set(ctx, rec.key(kindUnmarked, runComplete), nil),
			} {
				if err != nil {
					t.Fatal(err)
				}
			}
			if bc.block != nil {
				if err := bc.block(rec, state); err != nil {
					t.Fatal(err)
				}
			}
			if err := rec.SetState(ctx, state); err != nil {
				t.Fatal(err)
			}

			// Pop the front, then try the flip.
			c := NewCollector(nil, CollectorOptions{Frees: true})
			var res CollectResult
			if _, _, err := c.frontSlice(ctx, rec, &res, nil); err != nil {
				t.Fatal(err)
			}
			if err := c.flip(ctx, rec, &res); err != nil {
				t.Fatal(err)
			}
			held := bc.block != nil
			if held && (res.Popped != 0 || res.Flipped) {
				t.Fatalf("with %s, popped %d and flipped %v; want neither", bc.name, res.Popped, res.Flipped)
			}
			if !held && (res.Popped != 1 || !res.Flipped) {
				t.Fatalf("with netting done, popped %d and flipped %v; want the pop and the flip", res.Popped, res.Flipped)
			}
		})
	}
}
