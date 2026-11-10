package sobject_world_engine

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// headRoots returns the roots a walk or a mark of head starts from: the
// World root, then each retained root. It mirrors Hyperline's worldHeadRefs,
// which the block walk and the pack sweep's roots.txt read.
func headRoots(head *InnerState) []*block.BlockRef {
	out := []*block.BlockRef{head.GetHeadRef().GetRootRef()}
	for _, retained := range head.GetRetainedRoots() {
		out = append(out, retained.GetRootRef())
	}
	return out
}

// TestHeadRootsOmitPinsAndStages pins a root and writes a block through an
// open stage, then writes a new head, and checks what a mark of that head
// starts from. The store holds both, but neither is a root of the head: a
// reader pin and a stage live only in the local volume's ownership graph, and
// the head's RetainedRoots change only by SetRetainedRootOp. A mark from the
// head alone counts both as dead.
func TestHeadRootsOmitPinsAndStages(t *testing.T) {
	// Open a World over a store that owns roots and counts its pins.
	ctx := context.Background()
	c, so, head := newProcessTestWorld(t, ctx)
	store := &pinCountStore{testBlockStore: so.blockStore.(*testBlockStore), pins: map[string]int{}, complete: map[string]bool{}}
	so.blockStore = store

	// Pin a root a reader holds, and write a block through an open stage.
	pinned, _, err := store.PutBlock(ctx, []byte("a root a reader holds"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	releasePin, err := block.PinRoot(ctx, store, pinned)
	if err != nil {
		t.Fatal(err)
	}
	defer releasePin()
	stage, releaseStage, err := block.OpenStage(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStage()
	staged, _, err := stage.PutBlock(ctx, []byte("a block an open stage owns"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}

	// Write a new head: retain the current World under a name.
	opData, err := (&SOWorldOp{Body: &SOWorldOp_SetRetainedRoot{
		SetRetainedRoot: &SetRetainedRootOp{Name: "backup", RootRef: head.GetHeadRef().GetRootRef()},
	}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	next, res, err := c.processOp(ctx, c.le, newReplayWorld(c, store), opData, "test-op", newProcessTestPeerID(t), 1, 0, head)
	if err != nil {
		t.Fatal(err)
	}
	if !res.GetSuccess() || len(next.GetRetainedRoots()) != 1 {
		t.Fatalf("new head = %v, result %v", next, res)
	}

	// The store still holds both blocks, and the reader's pin.
	if store.pins[pinned.MarshalString()] != 1 {
		t.Fatalf("pins = %v", store.pins)
	}
	for name, ref := range map[string]*block.BlockRef{"pinned": pinned, "staged": staged} {
		if found, err := store.GetBlockExists(ctx, ref); err != nil || !found {
			t.Fatalf("%s block found=%v err=%v", name, found, err)
		}
	}

	// Neither is a root of the new head.
	for _, root := range headRoots(next) {
		if root.EqualVT(pinned) || root.EqualVT(staged) {
			t.Fatalf("head root %s is a pin or a stage", root.MarshalString())
		}
	}
	if got := len(headRoots(next)); got != 2 {
		t.Fatalf("head has %d roots, want the World and the one retained root", got)
	}
}
