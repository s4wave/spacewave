package sobject

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/sirupsen/logrus"
)

// TestMainDeviceHandoff checks that an owner makes its device the main device,
// that only the main device places operations, that another owner replaces a
// lost main device from the sequence it holds, and that positions the lost
// device signed after the replacement are ignored.
func TestMainDeviceHandoff(t *testing.T) {
	// Owners A and B and writer C share a Space. A makes itself the main
	// device, once.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers, SOParticipantRole_SOParticipantRole_OWNER, SOParticipantRole_SOParticipantRole_OWNER)
	ids := []string{peers[0].GetPeerID().String(), peers[1].GetPeerID().String(), peers[2].GetPeerID().String()}
	host, current := newTestSOHost(t.Context(), state)
	for i, want := range []bool{true, false} {
		changed, err := SetSOSequencer(t.Context(), host, ids[0], keys[0])
		if err != nil {
			t.Fatal(err)
		}
		if changed != want {
			t.Fatalf("appointment %d changed the sequencer: %v", i, changed)
		}
	}

	// B and C edit. Only A places their edits, and they become stable.
	b1 := writeTestOp(t, *current, keys[1], "b1")
	c1 := writeTestOp(t, *current, keys[2], "c1")
	if err := host.SequenceOperations(t.Context(), keys[1]); err != nil {
		t.Fatal(err)
	}
	if n := len((*current).GetSequence()); n != 0 {
		t.Fatalf("a device that is not the main device placed %d operations", n)
	}
	if n := mustOperationSet(t, *current).Unordered(); n != 2 {
		t.Fatalf("%d operations wait for the main device; want 2", n)
	}
	if err := host.SequenceOperations(t.Context(), keys[0]); err != nil {
		t.Fatal(err)
	}
	set := mustOperationSet(t, *current)
	if got := set.StablePoint(nil); len(got) != 2 || set.Unordered() != 0 {
		t.Fatalf("main device left %d operations stable and %d waiting; want b1 and c1 stable", len(got), set.Unordered())
	}

	// A places C's next edit, then goes offline before B receives that
	// position.
	c2 := writeTestOp(t, *current, keys[2], "c2")
	lost := (*current).CloneVT()
	stray, err := lost.SequenceOperations(mockSharedObjectID, keys[0])
	if err != nil || len(stray) != 1 {
		t.Fatalf("lost device placed %d operations, err %v", len(stray), err)
	}

	// B replaces A from the two positions it holds and places c2 itself.
	if _, err := SetSOSequencer(t.Context(), host, ids[1], keys[1]); err != nil {
		t.Fatal(err)
	}
	if start := (*current).GetConfig().GetSequencer().GetStart(); start.GetHeight() != 2 {
		t.Fatalf("replacement starts at height %d; want 2", start.GetHeight())
	}
	if err := host.SequenceOperations(t.Context(), keys[1]); err != nil {
		t.Fatal(err)
	}

	// A's late position arrives and changes nothing.
	tail := mustOperationSet(t, *current).SequenceTail()
	if _, err := (*current).AddSequence(mockSharedObjectID, stray[0]); err != nil {
		t.Fatal(err)
	}
	if got := mustOperationSet(t, *current).SequenceTail(); !got.EqualVT(tail) || bytes.Equal(got.GetHash(), stray[0].Hash()) {
		t.Fatal("the lost device's late position replaced the replacement's")
	}
	want := opHashes([]*SOOperation{b1, c1, c2})
	if got := mustOperationSet(t, *current).StablePoint(nil); !slices.EqualFunc(got, want, bytes.Equal) {
		t.Fatal("the replacement did not continue the sequence B held")
	}

	// Merge keeps the sequence B placed.
	if _, err := SetSOSequencer(t.Context(), host, "", keys[1]); err != nil {
		t.Fatal(err)
	}
	sequencer := (*current).GetConfig().GetSequencer()
	if sequencer.GetPeerId() != "" || sequencer.GetStart().GetHeight() != 3 {
		t.Fatalf("merge appointed %q from height %d", sequencer.GetPeerId(), sequencer.GetStart().GetHeight())
	}
}

// TestSequenceWatchesAppointment checks that the main device places
// operations when the state appoints it as the sequencer.
func TestSequenceWatchesAppointment(t *testing.T) {
	// A is the sequencer and B has edited.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	state.Config.Sequencer = &SOSequencer{PeerId: peers[0].GetPeerID().String()}
	writeTestOp(t, state, keys[1], "b1")

	// The watcher places B's edit as A.
	so := &ackTestObject{t: t, priv: keys[0], state: state, ctr: ccontainer.NewCContainer[SharedObjectStateSnapshot](nil)}
	so.publish()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err := Sequence(ctx, so, func(context.Context) error {
		defer cancel()
		_, err := state.SequenceOperations(mockSharedObjectID, keys[0])
		return err
	})
	if err != context.Canceled {
		t.Fatalf("sequencer ended with %v", err)
	}
	if n := len(state.GetSequence()); n != 1 {
		t.Fatalf("main device placed %d operations; want 1", n)
	}
}

// TestSequencerPlacesOwnOperations checks that the main device places its own
// edit in the write that adds it, and that a sequencing pass takes the provider
// lock only when an operation waits for a position.
func TestSequencerPlacesOwnOperations(t *testing.T) {
	// A is the sequencer. Count the host's provider locks.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	state.Config.Sequencer = &SOSequencer{PeerId: peers[0].GetPeerID().String()}
	host, current := newTestSOHost(t.Context(), state)
	lockFn, locks := host.lockFn, 0
	host.lockFn = func(ctx context.Context, sharedObjectID string) (SOStateLock, error) {
		locks++
		return lockFn(ctx, sharedObjectID)
	}

	// A's edit is placed as it is added, and the pass after it skips the lock.
	le := logrus.New().WithField("test", t.Name())
	if _, _, err := host.AddLocalOperation(t.Context(), le, testStepFactorySet(), keys[0], []byte("a1")); err != nil {
		t.Fatal(err)
	}
	if n := len((*current).GetSequence()); n != 1 {
		t.Fatalf("main device placed %d of its own operations; want 1", n)
	}
	if err := host.SequenceOperations(t.Context(), keys[0]); err != nil {
		t.Fatal(err)
	}
	if locks != 1 {
		t.Fatalf("took the provider lock %d times; want 1", locks)
	}

	// B's edit waits for a position, so the pass locks and places it.
	writeTestOp(t, *current, keys[1], "b1")
	if err := host.SequenceOperations(t.Context(), keys[0]); err != nil {
		t.Fatal(err)
	}
	if locks != 2 || len((*current).GetSequence()) != 2 {
		t.Fatalf("took %d locks and placed %d operations; want 2 and 2", locks, len((*current).GetSequence()))
	}
}
