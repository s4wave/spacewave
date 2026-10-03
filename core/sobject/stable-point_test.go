package sobject

import (
	"bytes"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
)

// TestStablePoint checks that the stable point waits for every roster member
// to build on an operation, that members acknowledge only edits, and that a
// checkpoint at the stable point keeps every later operation placed.
func TestStablePoint(t *testing.T) {
	// Owner A edits twice under writers B and C.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	roster := state.GetConfig().TrimRoster()
	a1 := writeTestOp(t, state, keys[0], "a1")
	a2 := writeTestOp(t, state, keys[0], "a2")
	set := mustOperationSet(t, state)

	// Nothing is stable until B and C build on A's edits, and only they lag.
	if got := set.StablePoint(roster); len(got) != 0 {
		t.Fatalf("stable point before acknowledgments: %d operations", len(got))
	}
	ids := []string{peers[0].GetPeerID().String(), peers[1].GetPeerID().String(), peers[2].GetPeerID().String()}
	if set.NeedsAcknowledgment(ids[0], ids[0], 1) || !set.NeedsAcknowledgment(ids[0], ids[1], 2) || set.NeedsAcknowledgment(ids[0], ids[1], 3) {
		t.Fatal("acknowledgment lag is not the unbuilt edit count")
	}

	// After B and C acknowledge, A's edits are stable and no acknowledgment
	// asks for another.
	writeTestOp(t, state, keys[1], "")
	writeTestOp(t, state, keys[2], "")
	set = mustOperationSet(t, state)
	if got := set.StablePoint(roster); !slices.EqualFunc(got, [][]byte{a1.Hash(), a2.Hash()}, bytes.Equal) {
		t.Fatalf("stable point %x; want a1, a2", got)
	}
	for _, id := range ids {
		if set.NeedsAcknowledgment(ids[0], id, 1) {
			t.Fatal("an acknowledgment asked for another")
		}
	}

	// An acknowledgment of the checkpointer asks B and C to answer at once.
	writeTestOp(t, state, keys[0], "")
	set = mustOperationSet(t, state)
	if set.NeedsAcknowledgment(ids[0], ids[0], 1) || !set.NeedsAcknowledgment(ids[0], ids[1], AcknowledgmentLag) {
		t.Fatal("the checkpointer's acknowledgment did not ask the members to answer")
	}
	writeTestOp(t, state, keys[1], "")
	writeTestOp(t, state, keys[2], "")
	set = mustOperationSet(t, state)
	for _, id := range ids {
		if set.NeedsAcknowledgment(ids[0], id, 1) {
			t.Fatal("an answer asked for another")
		}
	}

	// A checkpoint at the stable point keeps the answers placed.
	prefix := set.StablePoint(roster)
	checkpoint, err := state.BuildStableCheckpoint(mockSharedObjectID, keys[0], prefix, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AdoptCheckpoint(mockSharedObjectID, checkpoint); err != nil {
		t.Fatal(err)
	}
	if got := mustOperationSet(t, state).Order(); len(got) != 2 || len(state.GetOps()) != 2 {
		t.Fatalf("checkpoint left %d placed of %d held; want 2 of 2", len(got), len(state.GetOps()))
	}
}

// TestCheckpointCoverKeepsLinks checks that a checkpoint covering any prefix
// of the order keeps the rest of the order placed, even when a later
// operation names a covered operation that is not a head of the prefix.
func TestCheckpointCoverKeepsLinks(t *testing.T) {
	// Sign each operation with a fixed config hash.
	privA, _ := vectorKey(t, "cover a")
	privB, _ := vectorKey(t, "cover b")
	build := func(priv crypto.PrivKey, link *SOOperationLink) *SOOperation {
		// Sign the operation under a fixed config.
		t.Helper()
		link.ConfigHash = bytes.Repeat([]byte{3}, 32)
		op, err := BuildSOOperation(vectorObjectID, priv, []byte("data"), link, NewSOOperationLocalID())
		if err != nil {
			t.Fatal(err)
		}
		return op
	}

	// a1 <- a2 <- a3, with b1 naming a1 and b2 naming a2.
	a1 := build(privA, &SOOperationLink{Nonce: 1})
	a2 := build(privA, &SOOperationLink{Nonce: 2, PrevOpHash: a1.Hash()})
	a3 := build(privA, &SOOperationLink{Nonce: 3, PrevOpHash: a2.Hash()})
	b1 := build(privB, &SOOperationLink{Nonce: 1, Parents: []*SOOperationPosition{opPosition(t, a1)}})
	b2 := build(privB, &SOOperationLink{Nonce: 2, PrevOpHash: b1.Hash(), Parents: []*SOOperationPosition{opPosition(t, a2)}})

	// Hold them in one set.
	ops := []*SOOperation{a1, a2, a3, b1, b2}
	set := NewSOOperationSet(vectorObjectID, nil)
	for _, op := range ops {
		if _, err := set.Add(op); err != nil {
			t.Fatal(err)
		}
	}

	// Cover each prefix and replay the rest above the checkpoint.
	order := set.Order()
	for n := range order {
		above := NewSOOperationSet(vectorObjectID, &SOCheckpointInner{Authors: set.cover(order[:n])})
		for _, op := range ops {
			if _, err := above.Add(op); err != nil {
				t.Fatal(err)
			}
		}
		if got := above.Order(); !slices.EqualFunc(got, order[n:], bytes.Equal) {
			t.Fatalf("covering %d operations placed %d of %d", n, len(got), len(order)-n)
		}
	}
}

// mustOperationSet returns the operation set of state.
func mustOperationSet(t *testing.T, state *SOState) *SOOperationSet {
	t.Helper()
	set, err := state.OperationSet(mockSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	return set
}
