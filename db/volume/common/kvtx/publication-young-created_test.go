package kvtx

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// TestMinorPassKeepsFilledHoleACreatedParentNames checks that minor passes
// keep a filled hole whose only live namer is a parent block a write created
// and a root holds during the major pass that settled the hole. The catch-up
// reaches the hole from the adopt log with no namer, and the parent's list is
// in the created set, so no walk reads it and nameLater never records it.
// Promotion must keep the parent young, since a minor pass reaches the hole
// only through it once a write fills the hole.
func TestMinorPassKeepsFilledHoleACreatedParentNames(t *testing.T) {
	for _, w := range holeWriters {
		t.Run(w.name, func(t *testing.T) {
			// Release a block for the sweep.
			v, _ := newPublicationTestVolume(t)
			ctx := t.Context()
			rg := v.GetRefGraph()
			released := publicationFor(t, "released", "", "one")
			if err := v.PublishAtomic(ctx, released); err != nil {
				t.Fatal(err)
			}
			orphanPublication(t, v, released)

			// Pick a hole and a new parent whose list names it.
			hole, child := holeEntries(t)
			parent := publicationFor(t, "created-parent", "", "four").Entries[0]
			parent.Refs, parent.RefsKnown = []*block.BlockRef{hole.Ref}, true

			// A major pass in which one write stores the new parent and a
			// root holds it, then the pass settles the hole and promotes.
			var stats block_gc.Stats
			err := v.runBlockPass(ctx, rg, false, func(actual *transactionRefGraph) error {
				// Take the promote bound before the pass begins.
				var bound uint64
				err := actual.read(ctx, func(rg *block_gc.RefGraph) (err error) {
					bound, err = rg.YoungPosition(ctx)
					return err
				})
				if err != nil {
					return err
				}

				// Run the pass body while the pass is open.
				return v.inBlockPass(ctx, actual, func() error {
					// Mark the blocks the roots reach.
					mark, err := actual.MarkBlocksScan(ctx)
					if err != nil {
						return err
					}

					// Store the new parent, whose list names the hole.
					gs := block_gc.NewGCStoreOps(v, rg)
					opts := &block.PutOpts{ForceBlockRef: parent.Ref, Refs: parent.Refs, RefsKnown: true}
					if _, _, err := gs.PutBlock(ctx, parent.Data, opts); err != nil {
						return err
					}
					if err := gs.FlushPending(ctx); err != nil {
						return err
					}

					// Hold the new parent from the root.
					edge := block_gc.RefEdge{Subject: block_gc.NodeGCRoot, Object: block_gc.BlockIRI(parent.Ref)}
					if err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{edge}, nil); err != nil {
						return err
					}

					// Sweep, which settles the hole, and promote.
					complete, err := v.sweepUnreached(ctx, mark, &stats)
					if err != nil || !complete {
						t.Fatalf("complete=%v err=%v stats=%+v", complete, err, stats)
					}
					return v.promoteYoung(ctx, mark, bound)
				})
			})
			if err != nil {
				t.Fatal(err)
			}

			// The pass settled the hole and swept the released block.
			if stats.AbsentBlocks != 1 || stats.UnknownBlocks != 0 || stats.BlocksSwept != 1 {
				t.Fatalf("major pass stats=%+v", stats)
			}

			// Minor passes run before and after a write stores the hole, and
			// keep the live parent, the hole and its child once stored.
			minorPass := func(step string) {
				// Run one minor pass.
				t.Helper()
				var minor block_gc.Stats
				if err := v.SweepYoungBlocks(ctx, rg, &minor); err != nil {
					t.Fatal(err)
				}
				if minor.MinorPasses != 1 {
					t.Fatalf("%s: minor pass did not run: stats=%+v", step, minor)
				}

				// The parent stays, and the hole and its child survive once
				// stored.
				if _, found, err := v.GetBlock(ctx, parent.Ref); err != nil || !found {
					t.Fatalf("%s: live parent swept: found=%v err=%v", step, found, err)
				}
				for _, e := range []*block.PutBatchEntry{hole, child} {
					if _, found, err := v.GetBlock(ctx, e.Ref); err != nil {
						t.Fatal(err)
					} else if !found && step != "before fill" {
						t.Fatalf("%s: block a live parent names swept: stats=%+v", step, minor)
					}
				}
			}
			minorPass("before fill")
			w.write(t, v, rg, child, hole)
			minorPass("after fill")
			minorPass("after fill")
		})
	}
}
