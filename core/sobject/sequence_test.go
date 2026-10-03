package sobject

import (
	"bytes"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
)

// TestSequenceOrder checks that the sequence decides the order of concurrent
// operations, that it makes them stable without acknowledgments, that only
// the sequencer extends it, and that a checkpoint carries its head.
func TestSequenceOrder(t *testing.T) {
	// Owner A and writers B and C write concurrently under sequencer S.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	roster := state.GetConfig().TrimRoster()
	privS, idS := vectorKey(t, "sequencer s")
	state.Config.Sequencer = &SOSequencer{PeerId: idS}
	ops := []*SOOperation{concurrentOp(t, state, keys[0]), concurrentOp(t, state, keys[1]), concurrentOp(t, state, keys[2])}

	// Nothing is stable and nobody acknowledges before S places anything.
	set := mustOperationSet(t, state)
	if got := set.StablePoint(roster); len(got) != 0 {
		t.Fatalf("stable point before sequencing: %d operations", len(got))
	}
	if set.NeedsAcknowledgment(peers[0].GetPeerID().String(), peers[1].GetPeerID().String(), 1) {
		t.Fatal("a member acknowledged under a sequencer")
	}

	// S places them against hash order, and the order follows S.
	slices.SortFunc(ops, func(a, b *SOOperation) int { return bytes.Compare(b.Hash(), a.Hash()) })
	var head *SOSequenceHead
	for _, op := range ops {
		head = addTestSequence(t, state, privS, head, op)
	}
	want := opHashes(ops)
	set = mustOperationSet(t, state)
	if got := set.Order(); !slices.EqualFunc(got, want, bytes.Equal) {
		t.Fatal("order does not follow the sequence")
	}
	if got := set.StablePoint(roster); !slices.EqualFunc(got, want, bytes.Equal) {
		t.Fatal("sequenced operations are not stable")
	}

	// A position from anyone else above the start is not held.
	stray, err := BuildSOSequence(mockSharedObjectID, keys[0], head, opPosition(t, ops[0]))
	if err != nil {
		t.Fatal(err)
	}
	if added, err := state.AddSequence(mockSharedObjectID, stray); err != nil || added {
		t.Fatalf("held a position S did not sign: added %v, err %v", added, err)
	}

	// A later operation waits for S, and only S places it.
	d := writeTestOp(t, state, keys[1], "d")
	if got := mustOperationSet(t, state).StablePoint(roster); len(got) != 3 {
		t.Fatalf("unsequenced operation is stable: %d operations", len(got))
	}
	if added, err := state.SequenceOperations(mockSharedObjectID, keys[0]); err != nil || len(added) != 0 {
		t.Fatalf("a non-sequencer sequenced %d operations, err %v", len(added), err)
	}
	if added, err := state.SequenceOperations(mockSharedObjectID, privS); err != nil || len(added) != 1 {
		t.Fatalf("sequencer placed %d operations, err %v", len(added), err)
	}
	want = append(want, d.Hash())
	if got := mustOperationSet(t, state).StablePoint(roster); !slices.EqualFunc(got, want, bytes.Equal) {
		t.Fatal("sequenced operation is not stable")
	}
	if err := state.Validate(mockSharedObjectID); err != nil {
		t.Fatal(err)
	}

	// A checkpoint at the stable point covers the positions, and S continues
	// from its head.
	checkpoint, err := state.BuildStableCheckpoint(mockSharedObjectID, keys[0], want, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AdoptCheckpoint(mockSharedObjectID, checkpoint); err != nil {
		t.Fatal(err)
	}

	// The checkpoint carries the head and drops what it covers.
	inner, err := checkpoint.UnmarshalInner()
	if err != nil {
		t.Fatal(err)
	}
	if inner.GetSequence().GetHeight() != 4 || len(state.GetSequence()) != 0 || len(state.GetOps()) != 0 {
		t.Fatalf("checkpoint at sequence height %d left %d positions and %d operations", inner.GetSequence().GetHeight(), len(state.GetSequence()), len(state.GetOps()))
	}

	// S continues from the checkpoint's head.
	e := writeTestOp(t, state, keys[2], "e")
	added, err := state.SequenceOperations(mockSharedObjectID, privS)
	if err != nil || len(added) != 1 {
		t.Fatalf("sequencer placed %d operations after the checkpoint, err %v", len(added), err)
	}
	if got := mustOperationSet(t, state).StablePoint(roster); !slices.EqualFunc(got, [][]byte{e.Hash()}, bytes.Equal) {
		t.Fatal("sequence did not continue from the checkpoint")
	}
}

// TestSequenceFork checks that a sequencer signing two positions after one
// position places nothing past it.
func TestSequenceFork(t *testing.T) {
	// A and B write under sequencer S.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	privS, idS := vectorKey(t, "sequencer s")
	state.Config.Sequencer = &SOSequencer{PeerId: idS}

	// S signs both concurrent operations at height 1.
	a := concurrentOp(t, state, keys[0])
	b := concurrentOp(t, state, keys[1])
	addTestSequence(t, state, privS, nil, a)
	addTestSequence(t, state, privS, nil, b)

	// The fork stops the sequence, and the sequencer cannot extend it.
	set := mustOperationSet(t, state)
	if got := set.StablePoint(nil); len(got) != 0 {
		t.Fatalf("fork left %d operations stable", len(got))
	}
	if len(set.Order()) != 2 {
		t.Fatal("fork unplaced the operations")
	}
	if added, err := state.SequenceOperations(mockSharedObjectID, privS); err != nil || len(added) != 0 {
		t.Fatalf("sequencer extended a fork with %d positions, err %v", len(added), err)
	}
}

// TestSequenceHandoff checks that a new sequencer continues from its start,
// that positions the old one signed after the start are ignored, and that
// Merge after a start places the sequence first, then the rest by the roster
// rule.
func TestSequenceHandoff(t *testing.T) {
	// Three writers work under sequencer S1, with S2 standing by.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	privS1, idS1 := vectorKey(t, "sequencer s1")
	privS2, idS2 := vectorKey(t, "sequencer s2")
	state.Config.Sequencer = &SOSequencer{PeerId: idS1}

	// S1 places a and b, then c after the start a lost-device change names.
	ops := []*SOOperation{concurrentOp(t, state, keys[0]), concurrentOp(t, state, keys[1]), concurrentOp(t, state, keys[2])}
	slices.SortFunc(ops, func(a, b *SOOperation) int { return bytes.Compare(b.Hash(), a.Hash()) })
	start := addTestSequence(t, state, privS1, nil, ops[0])
	start = addTestSequence(t, state, privS1, start, ops[1])
	addTestSequence(t, state, privS1, start, ops[2])

	// S2 takes over at b. S1's c is ignored, and S2 places c itself.
	state.Config.Sequencer = &SOSequencer{PeerId: idS2, Start: start}
	if got := mustOperationSet(t, state).StablePoint(nil); len(got) != 2 {
		t.Fatalf("handoff kept %d positions; want 2", len(got))
	}
	if added, err := state.SequenceOperations(mockSharedObjectID, privS1); err != nil || len(added) != 0 {
		t.Fatalf("replaced sequencer placed %d operations, err %v", len(added), err)
	}
	if added, err := state.SequenceOperations(mockSharedObjectID, privS2); err != nil || len(added) != 1 {
		t.Fatalf("new sequencer placed %d operations, err %v", len(added), err)
	}
	want := opHashes(ops)
	if got := mustOperationSet(t, state).StablePoint(nil); !slices.EqualFunc(got, want, bytes.Equal) {
		t.Fatalf("new sequencer did not continue from the start: %x want %x", got, want)
	}

	// Merge from b keeps a and b first and orders c with the rest by hash,
	// stable once the roster builds on it.
	state.Config.Sequencer = &SOSequencer{Start: start}
	set := mustOperationSet(t, state)
	if got := set.Order(); !slices.EqualFunc(got[:2], want[:2], bytes.Equal) {
		t.Fatal("merge did not keep the sequence up to its start")
	}
	if set.NeedsAcknowledgment(peers[0].GetPeerID().String(), peers[1].GetPeerID().String(), 1) == false {
		t.Fatal("merge did not ask for acknowledgments")
	}
}

// concurrentOp signs and adds the first operation of priv's author, naming no
// other operation.
func concurrentOp(t *testing.T, state *SOState, priv crypto.PrivKey) *SOOperation {
	// Sign the author's first operation and add it.
	t.Helper()
	op, err := BuildSOOperation(mockSharedObjectID, priv, nil, linkAt(nil, priv, 1), NewSOOperationLocalID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddOperation(mockSharedObjectID, op); err != nil {
		t.Fatal(err)
	}
	return op
}

// addTestSequence signs, as priv, the position after prev placing op, adds it
// and returns its head.
func addTestSequence(t *testing.T, state *SOState, priv crypto.PrivKey, prev *SOSequenceHead, op *SOOperation) *SOSequenceHead {
	// Sign the position and add it.
	t.Helper()
	record, err := BuildSOSequence(mockSharedObjectID, priv, prev, opPosition(t, op))
	if err != nil {
		t.Fatal(err)
	}
	if added, err := state.AddSequence(mockSharedObjectID, record); err != nil || !added {
		t.Fatalf("add sequence: added %v, err %v", added, err)
	}
	return &SOSequenceHead{Height: prev.GetHeight() + 1, Hash: record.Hash()}
}

// opHashes returns the hashes of ops, in order.
func opHashes(ops []*SOOperation) [][]byte {
	out := make([][]byte, len(ops))
	for i, op := range ops {
		out[i] = op.Hash()
	}
	return out
}
