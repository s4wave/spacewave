package block_gc

import (
	"context"
	"strconv"
	"testing"
	"time"

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
// runs while the free could not: a paged release in progress (the model's
// FlipSettled, mutant rps-noflipsettled), an adoption younger than the grace
// period, or a rewritten list awaiting its recount. Each blocker leaves a
// listed block at zero, queued at a ripe sequence the up cursor passed, so
// the free proof would pop it and the flip would follow, unless the check
// holds; otherwise the free parks it, which the flip requires. Collect nets
// before the settle runs, so no writer reaches these states with work owed;
// the test writes them as a netting step cut short would.
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
		{"a paged release", func(_ *Records, state *LivenessState) error {
			state.Release = &Release{Block: key}
			return nil
		}},
		{"an adoption younger than the grace period", func(_ *Records, state *LivenessState) error {
			state.AdoptAtUnixMilli = time.Now().UnixMilli()
			return nil
		}},
		{"a list rewrite awaiting a recount", func(rec *Records, _ *LivenessState) error {
			return rec.tx.Set(ctx, rec.key(kindRewritten), nil)
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
				UpCursor:   10,
			}
			for _, err := range []error{
				rec.PutChildren(ctx, ref, nil),
				rec.SetHolders(ctx, ref, 0, 5),
				tx.Set(ctx, rec.queueKey(5, key), nil),
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

			// Pop the front, park what the free finds, then try the flip.
			c := NewCollector(nil, CollectorOptions{Frees: true})
			var res CollectResult
			if _, _, err := c.frontSlice(ctx, rec, nil, &res, frontPos{}); err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := c.freeSlice(ctx, rec, nil, &res, nil); err != nil {
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

// TestWalkRestartsUnreadableProgress checks that a walk whose scratch holds a
// progress record this build cannot read, as a build with another WalkMark
// leaves, drops it with the run's records and begins the run again, instead
// of failing every open until someone deletes the store.
func TestWalkRestartsUnreadableProgress(t *testing.T) {
	// Leave a progress record that does not decode, beside an edge.
	ctx := context.Background()
	scratch := store_kvtx_inmem.NewStore()
	tx, err := scratch.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	edge := walkKey(walkEdge, []byte("stale edge"))
	for key, value := range map[string][]byte{string([]byte{walkProgressKind}): {0xff, 0xff}, string(edge): nil} {
		if err := tx.Set(ctx, []byte(key), value); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Open the run: it begins again and keeps nothing of the old one.
	w, err := openWalk(ctx, nil, scratch, WalkOptions{Run: "next"})
	if err != nil {
		t.Fatalf("open over unreadable progress: %v", err)
	}
	if !w.res.Restarted || w.prog.GetPhase() != WalkPhase_WALK_PHASE_ROOTS {
		t.Fatalf("restarted %v in phase %v; want a new run", w.res.Restarted, w.prog.GetPhase())
	}
	prog, err := w.readProgress(ctx)
	if err != nil || prog.GetRun() != "next" {
		t.Fatalf("progress %v, %v; want the new run's", prog, err)
	}
	tx, err = scratch.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if _, found, err := tx.Get(ctx, edge); err != nil || found {
		t.Fatalf("stale edge found %v, %v; want it cleared", found, err)
	}
}
