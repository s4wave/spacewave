package sobject

import (
	"slices"
	"sync"
	"testing"
)

// TestSOStateClone checks immutable operation sharing and independent state edits.
func TestSOStateClone(t *testing.T) {
	// Copy every state field, including an unknown field retained on the wire.
	state := &SOState{
		Config:          &SharedObjectConfig{ConfigChainHash: []byte("config")},
		Checkpoint:      &SOCheckpoint{Inner: []byte("checkpoint")},
		KeyEpochs:       []*SOKeyEpoch{{Epoch: 1}},
		Ops:             []*SOOperation{{Inner: []byte("first")}, {Inner: []byte("second")}},
		Invites:         []*SOInvite{{}},
		Sequence:        []*SOSequence{{Inner: []byte("sequence")}},
		ControlMessages: []*SOControlMessage{{Inner: []byte("control")}},
		unknownFields:   []byte{0x40, 1},
	}
	before := state.CloneVT()
	next := state.Clone()
	if !next.EqualVT(before) || next.Ops[0] != state.Ops[0] || next.Ops[1] != state.Ops[1] || next.Checkpoint != state.Checkpoint {
		t.Fatal("copy changed state or copied immutable operations or checkpoint")
	}

	// Mutating the state copy never changes an earlier snapshot.
	next.Config.ConfigChainHash[0]++
	next.Checkpoint = &SOCheckpoint{Inner: []byte("replaced")}
	next.KeyEpochs[0].Epoch++
	next.Invites[0].Uses++
	next.Sequence[0].Inner[0]++
	next.ControlMessages[0].Inner[0]++
	next.unknownFields[1]++

	// Replace operation membership without editing any shared operation.
	next.Ops = slices.Delete(next.Ops, 0, 1)
	next.Ops = append(next.Ops, &SOOperation{Inner: []byte("third")})
	if !state.EqualVT(before) {
		t.Fatal("editing the copy changed the original state")
	}

	// Nil snapshots remain nil, and deep copies still isolate operation edits.
	if (*SOState)(nil).Clone() != nil {
		t.Fatal("copy of nil state is nonnil")
	}
	before.Ops[0].Inner[0]++
	if state.Ops[0].Inner[0] == before.Ops[0].Inner[0] {
		t.Fatal("deep copy shares operation bytes")
	}
}

// TestSOStateCloneSharedOperationEdits checks that the edits a state copy makes
// to its operation list never reach the operations it shares.
func TestSOStateCloneSharedOperationEdits(t *testing.T) {
	// Hold one operation from each of an owner and a writer, then copy them.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	writeTestOp(t, state, keys[0], "a")
	writeTestOp(t, state, keys[1], "b")
	before := state.CloneVT()
	held := slices.Clone(state.Ops)

	// Add an operation to the copy, then let a checkpoint filter the copy's
	// list in place and clear its tail.
	next := state.Clone()
	checkpoint, err := next.BuildNextCheckpoint(mockSharedObjectID, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	later := writeTestOp(t, next, keys[1], "c")
	if err := next.AdoptCheckpoint(mockSharedObjectID, checkpoint); err != nil {
		t.Fatal(err)
	}
	if len(next.Ops) != 1 || next.Ops[0] != later {
		t.Fatalf("copy holds %d operations; want only the later one", len(next.Ops))
	}

	// The original keeps its list, its operations and their verification.
	if !slices.Equal(state.Ops, held) || !state.EqualVT(before) {
		t.Fatal("editing the copy changed the original state")
	}
	if err := state.Validate(mockSharedObjectID); err != nil {
		t.Fatal(err)
	}
}

// TestSOStateCloneConcurrentSnapshots checks that copies sharing operations
// can be edited and verified at once. Run under the race detector.
func TestSOStateCloneConcurrentSnapshots(t *testing.T) {
	// Share the held operations among copies behind one verifier.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	writeTestOp(t, state, keys[0], "a")
	writeTestOp(t, state, keys[1], "b")
	checkpoint, err := state.BuildNextCheckpoint(mockSharedObjectID, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	verifier := NewSOOperationVerifier(mockSharedObjectID)

	// Adopt the checkpoint in each copy while every copy builds operation sets.
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			// Read the shared operations through the verifier.
			next := state.Clone()
			if _, err := verifier.OperationSet(state); err != nil {
				t.Error(err)
			}

			// Filter only the copy's list, then read the copy.
			if err := next.AdoptCheckpoint(mockSharedObjectID, checkpoint); err != nil {
				t.Error(err)
			}
			if _, err := verifier.OperationSet(next); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if len(state.Ops) != 2 {
		t.Fatalf("original holds %d operations; want 2", len(state.Ops))
	}
}

// BenchmarkStateSnapshot measures copying a state and rebuilding its operation
// set through the verifier used across published snapshots.
func BenchmarkStateSnapshot(b *testing.B) {
	// Hold the same signed operations throughout the measurement.
	state, _ := newBenchState(b, 1024)

	// Measure a state copy separately from the cached set construction.
	b.Run("Clone", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if next := state.Clone(); !next.EqualVT(state) {
				b.Fatal("copy changed the state")
			}
		}
	})
	b.Run("OperationSet", func(b *testing.B) {
		// Prime the verifier before timing unchanged snapshots.
		verifier := NewSOOperationVerifier(mockSharedObjectID)
		if _, err := verifier.OperationSet(state); err != nil {
			b.Fatal(err)
		}

		// Build each snapshot with the already verified operations.
		b.ReportAllocs()
		for b.Loop() {
			set, err := verifier.OperationSet(state)
			if err != nil {
				b.Fatal(err)
			}
			if set.Len() != len(state.GetOps()) {
				b.Fatal("set lost operations")
			}
		}
	})
}
