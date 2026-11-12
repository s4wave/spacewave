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

// TestAdmitAppendsPendingWorlds checks that each admission in a stored epoch
// appends one block naming its candidate to the pending root, linked to the
// earlier block, rather than rewriting a block naming every pending World:
// the write under the root lock stays one root whatever the set holds.
func TestAdmitAppendsPendingWorlds(t *testing.T) {
	ctx := t.Context()
	a, _ := newVolumeAdapter(t, 4<<10, time.Hour)
	a.replay = &spaceReplay{a: a, base: &InnerState{}}
	for i := range 3 {
		// Admit a candidate whose page goes past the bound.
		page := []byte("page " + string(rune('a'+i)))
		page = bytes.Repeat(page, 1<<10)
		pageRef, err := block.BuildBlockRef(page, nil)
		if err != nil {
			t.Fatal(err)
		}
		data := []byte("candidate World " + string(rune('a'+i)))
		root, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		adm := a.newAdmission(world.Candidate{Root: root, Entries: []*block.PutBatchEntry{
			{Ref: pageRef, Data: page, RefsKnown: true},
			{Ref: root, Data: data, Refs: []*block.BlockRef{pageRef}, RefsKnown: true},
		}}, nil, nil)
		a.admit(adm)
		if err := a.storeAdmitted(ctx, adm); err != nil {
			t.Fatal(err)
		}

		// The pending root's newest block names this candidate alone.
		if len(a.pendingWorlds.roots) != i+1 || a.pendingWorlds.links != i+1 {
			t.Fatalf("admission %d: pending root holds %d Worlds in %d blocks", i, len(a.pendingWorlds.roots), a.pendingWorlds.links)
		}
		held, found, err := a.pages.GetBlock(ctx, a.held[RootPending])
		if err != nil || !found {
			t.Fatalf("admission %d: read the pending root block: %v %v", i, found, err)
		}
		msg := &HeldRoots{}
		if err := msg.UnmarshalVT(held); err != nil {
			t.Fatal(err)
		}
		if len(msg.GetRoots()) != 1 || !msg.GetRoots()[0].EqualsRef(root) {
			t.Fatalf("admission %d: the pending root block names %d Worlds", i, len(msg.GetRoots()))
		}
	}
}

// TestRootSetMoveKeepsLostRoot checks that a root set whose stored block names
// a World whose block was lost still moves: a trim that keeps the lost World,
// an append and a rewrite at the chain bound land, since the child check
// passes the roots the set held already. A move that adds a lost World is
// still refused.
func TestRootSetMoveKeepsLostRoot(t *testing.T) {
	ctx := t.Context()
	a, _ := newVolumeAdapter(t, 4<<10, time.Hour)
	stored := func(data string) *block.BlockRef {
		t.Helper()
		ref, _, err := a.pages.StoreOps.PutBlock(ctx, []byte(data), &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	lost := func(data string) *block.BlockRef {
		t.Helper()
		ref, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}

	// Store a pending root naming a lost World and a stored one, past the
	// child check, as a set that named the World before its block was lost.
	hole, kept := lost("lost World"), stored("kept World")
	entry, err := refsEntry(&HeldRoots{Roots: []*block.BlockRef{hole, kept}}, []*block.BlockRef{hole, kept})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.pages.StoreOps.SetRoots(ctx, []*block.PutBatchEntry{entry}, []block.NamedRoot{{Name: RootPending, Ref: entry.Ref}}); err != nil {
		t.Fatal(err)
	}
	a.held[RootPending] = entry.Ref
	a.pendingWorlds.roots, a.pendingWorlds.links = []*block.BlockRef{hole, kept}, 1
	release, err := a.rootMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// A trim keeping the lost World, an append, and a rewrite at the bound.
	h := &a.pendingWorlds
	if err := a.setRootSets(ctx, []*rootSet{h}, [][]*block.BlockRef{{hole}}, nil); err != nil {
		t.Fatalf("trim keeping a lost World: %v", err)
	}
	if err := a.appendRoots(ctx, h, []*block.BlockRef{stored("appended World")}); err != nil {
		t.Fatalf("append beside a lost World: %v", err)
	}
	h.links = maxRootLinks
	if err := a.appendRoots(ctx, h, []*block.BlockRef{stored("rewritten World")}); err != nil {
		t.Fatalf("rewrite at the bound keeping a lost World: %v", err)
	}
	if h.links != 1 || !h.holds(hole) {
		t.Fatalf("the rewrite left %d blocks, holding the lost World %v", h.links, h.holds(hole))
	}

	// Adding a lost World is refused.
	if err := a.appendRoots(ctx, h, []*block.BlockRef{lost("another lost World")}); !errors.Is(err, block.ErrMissingChild) {
		t.Fatalf("append of a lost World: %v, want %v", err, block.ErrMissingChild)
	}
}
