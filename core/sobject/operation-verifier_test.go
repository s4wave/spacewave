package sobject

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// newBenchState returns a valid state holding n operations of its owner's
// chain, and the owner's key.
func newBenchState(b *testing.B, n int) (*SOState, crypto.PrivKey) {
	// Start from a valid state with one owner.
	b.Helper()
	peers := createMockPeers(b, 1)
	owner := mustPrivKeys(b, peers)[0]
	state, _ := newTestSOState(b, peers)

	// Sign each operation after its predecessor.
	var prev []byte
	for nonce := uint64(1); nonce <= uint64(n); nonce++ {
		link := &SOOperationLink{Nonce: nonce, PrevOpHash: prev, ConfigHash: state.GetConfig().GetConfigChainHash()}
		op, err := BuildSOOperation(mockSharedObjectID, owner, []byte("data"), link, NewSOOperationLocalID())
		if err != nil {
			b.Fatal(err)
		}
		if _, err := state.AddOperation(mockSharedObjectID, op); err != nil {
			b.Fatal(err)
		}
		prev = op.Hash()
	}
	return state, owner
}

// BenchmarkCommit measures one local write above a state of n operations: the
// host adds the operation under its lock, then the published snapshot of the
// written state reads its operation set.
func BenchmarkCommit(b *testing.B) {
	for _, n := range []int{32, 256, 1024} {
		b.Run(fmt.Sprintf("ops=%d", n), func(b *testing.B) {
			// Hold the state behind a lock that writes it back.
			base, owner := newBenchState(b, n)
			peerID, err := peer.IDFromPrivateKey(owner)
			if err != nil {
				b.Fatal(err)
			}
			current := base
			host := NewSOHost(nil, nil, func(context.Context, string) (SOStateLock, error) {
				return NewSOStateLock(current, func(_ context.Context, next *SOState, _ ...*SOConfigChange) error {
					current = next
					return nil
				}, func() {}), nil
			}, mockSharedObjectID)
			le := logrus.New().WithField("bench", b.Name())

			// Write one operation above the base state each iteration.
			b.ReportAllocs()
			for b.Loop() {
				current = base
				localID, _, err := host.AddLocalOperation(b.Context(), le, testStepFactorySet(), owner, []byte("data"))
				if err != nil {
					b.Fatal(err)
				}
				snap := NewSOStateParticipantHandle(le, testStepFactorySet(), mockSharedObjectID, current, owner, peerID).
					WithOperationVerifier(host.GetOperationVerifier())
				set, err := snap.GetOperationSet(b.Context())
				if err != nil {
					b.Fatal(err)
				}
				if set.Find(peerID.String(), localID) == nil || set.Len() != n+1 {
					b.Fatal("written operation missing from the set")
				}
			}
		})
	}
}


// requireSameSet fails unless got holds the operations, order, heads and
// evidence of want.
func requireSameSet(t *testing.T, got, want *SOOperationSet) {
	// Compare the operations, their order and the heads.
	t.Helper()
	if got.Len() != want.Len() {
		t.Fatalf("set holds %d operations; a rebuild holds %d", got.Len(), want.Len())
	}
	if !slices.EqualFunc(got.Order(), want.Order(), bytes.Equal) {
		t.Fatal("set orders its operations differently from a rebuild")
	}
	if !slices.EqualFunc(headHashes(got.Heads()), headHashes(want.Heads()), bytes.Equal) {
		t.Fatal("set has other heads than a rebuild")
	}

	// Compare each body and the evidence.
	for _, h := range want.Order() {
		if !got.Get(h).EqualVT(want.Get(h)) {
			t.Fatalf("set holds another body for operation %x", h)
		}
	}
	if len(got.Equivocations()) != len(want.Equivocations()) || got.Unordered() != want.Unordered() {
		t.Fatal("set has other evidence than a rebuild")
	}
}

// TestSOOperationVerifier checks that the set a verifier builds equals a
// rebuild from nothing after operations are added, a checkpoint covers them
// and another state replaces the held one, and that it verifies only the
// operations it has not seen unchanged.
func TestSOOperationVerifier(t *testing.T) {
	// Build each set through one verifier and compare it with a rebuild.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	verifier := NewSOOperationVerifier(mockSharedObjectID)
	build := func(state *SOState) *SOOperationSet {
		// Build the set through the verifier and from nothing.
		t.Helper()
		got, err := verifier.OperationSet(state)
		if err != nil {
			t.Fatal(err)
		}
		want, err := state.OperationSet(mockSharedObjectID)
		if err != nil {
			t.Fatal(err)
		}

		// The sets agree and the verifier holds the state's operations alone.
		requireSameSet(t, got, want)
		if len(verifier.verified) != len(state.GetOps()) {
			t.Fatalf("verifier remembers %d operations; the state holds %d", len(verifier.verified), len(state.GetOps()))
		}
		return got
	}

	// Adding an operation verifies it alone: the held bodies are reused.
	prev := build(state)
	for i := range 6 {
		writeTestOp(t, state, keys[i%len(keys)], "data")
		set := build(state)
		for _, h := range prev.Order() {
			if set.Get(h) != prev.Get(h) {
				t.Fatalf("write %d verified operation %x again", i, h)
			}
		}
		prev = set
	}

	// A checkpoint covers the operations and the verifier forgets them.
	checkpoint, err := state.BuildNextCheckpoint(mockSharedObjectID, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AdoptCheckpoint(mockSharedObjectID, checkpoint); err != nil {
		t.Fatal(err)
	}
	if set := build(state); set.Len() != 0 {
		t.Fatalf("set holds %d operations above the checkpoint", set.Len())
	}
	writeTestOp(t, state, keys[1], "data")
	build(state)

	// Another state replaces the held one, and back.
	fork := state.CloneVT()
	writeTestOp(t, fork, keys[2], "data")
	build(fork)
	build(state)
	build(fork)
	build(&SOState{Config: state.GetConfig(), Checkpoint: state.GetCheckpoint()})
}

// TestSOOperationVerifierSignature checks that an operation whose signature
// changed is verified again and rejected, as in a rebuild.
func TestSOOperationVerifierSignature(t *testing.T) {
	// Verify two operations.
	peers := createMockPeers(t, 1)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	writeTestOp(t, state, keys[0], "a")
	writeTestOp(t, state, keys[0], "b")
	verifier := NewSOOperationVerifier(mockSharedObjectID)
	if _, err := verifier.OperationSet(state); err != nil {
		t.Fatal(err)
	}

	// Sign the first one's body with the second one's signature.
	forged := state.CloneVT()
	forged.Ops[0].Signature = forged.Ops[1].Signature.CloneVT()
	if _, err := verifier.OperationSet(forged); err == nil {
		t.Fatal("verifier accepted an operation with another signature")
	}
	if _, err := forged.OperationSet(mockSharedObjectID); err == nil {
		t.Fatal("rebuild accepted an operation with another signature")
	}
}
