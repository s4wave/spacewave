//go:build !goscript

package space_world

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// TestAdmitHoldsStoredBlocks checks that a block of a candidate stored past
// the PendingStore's bound outlives every stage that put it, a collection at
// a 1 ns grace, and another writer's put that carries the stages' releases
// into the Volume: Admit holds the candidate whenever a block of it went to
// the Volume, whether its own put stored the block or another stage did, such
// as the writer's transaction draining early or a World stage whose build the
// candidate adopts, while the candidate's own entries fit. A sweep between
// the put and the hold drops nothing the candidate reaches, and an admission
// failed there leaves no stage counted.
func TestAdmitHoldsStoredBlocks(t *testing.T) {
	errCancel := errors.New("cancel between put and hold")
	for _, tc := range []struct {
		name string
		// writerStage stores the page through a stage of its own.
		writerStage bool
		// atPut runs at the put step.
		atPut func(a *Adapter) error
	}{
		{name: "admit-put"},
		{name: "writer-stage", writerStage: true},
		{name: "sweep", writerStage: true, atPut: func(a *Adapter) error { a.sweep(); return nil }},
		{name: "cancel", writerStage: true, atPut: func(*Adapter) error { return errCancel }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			a, collect := newVolumeAdapter(t, 4<<10, time.Nanosecond)
			a.replay = &spaceReplay{a: a, base: &InnerState{}}
			entry := func(data []byte, refs ...*block.BlockRef) *block.PutBatchEntry {
				t.Helper()
				ref, err := block.BuildBlockRef(data, nil)
				if err != nil {
					t.Fatal(err)
				}
				return &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}
			}

			// Store a page past the bound, through another stage or the
			// candidate's own entries, under a root that fits.
			page := entry(bytes.Repeat([]byte("page "), 2<<10))
			root := entry([]byte("candidate World"), page.Ref)
			entries := []*block.PutBatchEntry{page, root}
			releaseWriter := func() {}
			if tc.writerStage {
				stage, release, err := a.pages.OpenStage(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := stage.PutBlockBatch(ctx, []*block.PutBatchEntry{page}); err != nil {
					t.Fatal(err)
				}
				entries, releaseWriter = []*block.PutBatchEntry{root}, release
			}

			// Admit the candidate, end the other stage, let another writer's
			// put release the stages' roots, and collect.
			if tc.atPut != nil {
				hook := func(step string) error {
					if step == admitStepPut {
						return tc.atPut(a)
					}
					return nil
				}
				admitStepHook.Store(&hook)
				defer admitStepHook.Store(nil)
			}
			adm := a.newAdmission(world.Candidate{Root: root.Ref, Entries: entries}, nil, nil)
			a.admit(adm)
			err := a.storeAdmitted(ctx, adm)
			if err != nil {
				a.fail(adm, err)
			}
			releaseWriter()
			if _, _, err := a.pages.StoreOps.PutBlock(ctx, []byte("another writer"), &block.PutOpts{RefsKnown: true}); err != nil {
				t.Fatal(err)
			}
			collect()

			// A failed admission leaves no stage counted and no hold.
			if errors.Is(err, errCancel) {
				if a.pages.StagesStored() || len(a.pendingWorlds.roots) != 0 {
					t.Fatalf("a failed admission left stages stored %v, pending Worlds %d", a.pages.StagesStored(), len(a.pendingWorlds.roots))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			// The page must still read.
			found, err := a.pages.StoreOps.GetBlockExists(ctx, page.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatal("a collection freed a page of the admitted candidate")
			}
		})
	}
}
