package sobject

import (
	"math"
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

// TestStateNonceMonotonicity checks that root updates and result clearing cannot
// make a peer reuse a nonce that the state has already observed.
func TestStateNonceMonotonicity(t *testing.T) {
	// Start with a signed root and a committed operation from the owner.
	peers := createMockPeers(t, 1)
	priv, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	peerID := peers[0].GetPeerID().String()
	base := createMockSOState(peers, nil)
	base.Root = createMockSORoot(t, 1, peers[0])

	// A later signed root must retain every committed nonce.
	for _, omit := range []bool{false, true} {
		t.Run(map[bool]string{false: "lower committed nonce", true: "omitted committed nonce"}[omit], func(t *testing.T) {
			// Sign a root holding a head at nonce 5, then a successor that lowers or drops it.
			state := base.CloneVT()
			state.Root.AccountNonces = []*SOAccountNonce{{PeerId: peerID, Nonce: 5, OpHash: mockPrevOpHash}}
			state.Root.ValidatorSignatures = nil
			if err := state.Root.SignInnerData(priv, mockSharedObjectID, 1, hash.RecommendedHashType); err != nil {
				t.Fatal(err)
			}
			next := createMockSORoot(t, 2, peers[0])
			if omit {
				next.AccountNonces = nil
				next.ValidatorSignatures = nil
				if err := next.SignInnerData(priv, mockSharedObjectID, 2, hash.RecommendedHashType); err != nil {
					t.Fatal(err)
				}
			}
			if err := state.UpdateRootState(mockSharedObjectID, next, "", nil, nil); err == nil {
				t.Fatal("accepted a root that loses a committed nonce")
			}
		})
	}

	// The root's head for a rejected operation outlives clearing the rejection.
	t.Run("clear remote rejection", func(t *testing.T) {
		// Reject the author's operation at nonce 7.
		state := base.CloneVT()
		localID := NewSOOperationLocalID()
		rejection, err := BuildSOOperationRejection(priv, mockSharedObjectID,
			peers[0].GetPeerID(), 7, localID, nil)
		if err != nil {
			t.Fatal(err)
		}

		// The root names the rejected operation as the author head.
		root := createMockSORoot(t, 2, peers[0])
		root.AccountNonces = []*SOAccountNonce{{PeerId: peerID, Nonce: 7, OpHash: mockPrevOpHash}}
		root.ValidatorSignatures = nil
		if err := root.SignInnerData(priv, mockSharedObjectID, 2, hash.RecommendedHashType); err != nil {
			t.Fatal(err)
		}
		if err := state.UpdateRootState(mockSharedObjectID, root, "",
			[]*SOOperationRejection{rejection}, nil); err != nil {
			t.Fatal(err)
		}

		// The next link follows the rejected nonce.
		before := state.NextOperationLink(peerID).Nonce
		if before != 8 {
			t.Fatalf("rejected nonce 7 requires next nonce 8, got %d", before)
		}

		// Clearing the rejection keeps the head.
		clear, err := BuildSOClearOperationResult(mockSharedObjectID, priv, localID)
		if err != nil {
			t.Fatal(err)
		}
		if err := state.ClearOperationResult(mockSharedObjectID, clear); err != nil {
			t.Fatal(err)
		}
		if after := state.NextOperationLink(peerID).Nonce; after < before {
			t.Fatalf("clearing rejection lowered next nonce from %d to %d", before, after)
		}
		if err := state.Validate(mockSharedObjectID); err != nil {
			t.Fatal(err)
		}
	})

	// Explicitly accepted batches retain their consumed nonce even before the
	// root's account projection includes that operation.
	t.Run("accepted batch ahead of root projection", func(t *testing.T) {
		state := base.CloneVT()
		op := signedLeanOperation(t, priv, peerID, linkAt(nil, priv, 9), NewSOOperationLocalID())
		if err := state.UpdateRootState(mockSharedObjectID, createMockSORoot(t, 2, peers[0]), "",
			nil, []*SOOperation{op}); err != nil {
			t.Fatal(err)
		}
		if got := state.NextOperationLink(peerID).Nonce; got != 10 {
			t.Fatalf("accepted nonce 9 requires next nonce 10, got %d", got)
		}
	})

	// An author whose chain reached the last sequence can write no more.
	t.Run("exhausted chain", func(t *testing.T) {
		state := base.CloneVT()
		state.Root.AccountNonces = []*SOAccountNonce{{PeerId: peerID, Nonce: math.MaxUint64, OpHash: mockPrevOpHash}}
		if got := state.NextOperationLink(peerID).Nonce; got != 0 {
			t.Fatalf("exhausted account returned to usable nonce %d", got)
		}
		for _, nonce := range []uint64{0, 1, math.MaxUint64} {
			op := signedLeanOperation(t, priv, peerID, linkAt(state, priv, nonce), NewSOOperationLocalID())
			if err := state.QueueOperation(mockSharedObjectID, op); err == nil {
				t.Fatalf("exhausted account accepted nonce %d", nonce)
			}
		}
	})
}

// TestStateOperationSigner prevents an authorized writer from using another
// account's nonce and local-ID namespace.
func TestStateOperationSigner(t *testing.T) {
	// Use two peers and the owner key.
	peers := createMockPeers(t, 2)
	priv, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	state := createMockSOState(peers, nil)
	state.Root = createMockSORoot(t, 1, peers[0])

	// The owner cannot sign under the other peer's account.
	op := signedLeanOperation(t, priv, peers[1].GetPeerID().String(), linkAt(state, priv, 1), NewSOOperationLocalID())
	if err := state.QueueOperation(mockSharedObjectID, op); err == nil {
		t.Fatal("writer queued an operation under another account")
	}
	state.Ops = []*SOOperation{op}
	if err := state.Validate(mockSharedObjectID); err == nil {
		t.Fatal("validated an operation attributed to a different signer")
	}
}

// TestStateOperationIdentity checks that validation rejects ambiguous local IDs
// and that queuing preserves a validated state's per-peer nonce order.
func TestStateOperationIdentity(t *testing.T) {
	// Use real signatures so each counterexample isolates operation identity.
	peers := createMockPeers(t, 1)
	priv, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	state := createMockSOState(peers, nil)
	state.Root = createMockSORoot(t, 1, peers[0])

	// Sign two operations that share a local ID.
	localID := NewSOOperationLocalID()
	first, err := BuildSOOperation(mockSharedObjectID, priv, []byte("one"), linkAt(state, priv, 1), localID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildSOOperation(mockSharedObjectID, priv, []byte("two"), linkAt(state, priv, 2), localID)
	if err != nil {
		t.Fatal(err)
	}
	state.QueuedAccountNonces = []*SOAccountNonce{{PeerId: peers[0].GetPeerID().String(), Nonce: 2, OpHash: mockPrevOpHash}}

	// Different nonces cannot make one local operation ID refer to two writes.
	t.Run("duplicate pending local ID", func(t *testing.T) {
		candidate := state.CloneVT()
		candidate.Ops = []*SOOperation{first, second}
		if err := candidate.Validate(mockSharedObjectID); err == nil {
			t.Fatal("validated two queued operations with one local ID")
		}
	})

	// A rejection and a pending operation cannot share a local ID or nonce.
	t.Run("pending and rejected", func(t *testing.T) {
		candidate := state.CloneVT()
		candidate.Ops = []*SOOperation{first}
		rejection, err := BuildSOOperationRejection(priv, mockSharedObjectID, peers[0].GetPeerID(), 2, localID, nil)
		if err != nil {
			t.Fatal(err)
		}
		candidate.OpRejections = []*SOPeerOpRejections{{PeerId: peers[0].GetPeerID().String(), Rejections: []*SOOperationRejection{rejection}}}
		if err := candidate.Validate(mockSharedObjectID); err == nil {
			t.Fatal("validated an operation that is both queued and rejected")
		}
	})

	// A validated imported queue must contribute to nonce selection even when
	// its separately stored reservation has not been reconstructed yet.
	t.Run("pending nonce without reservation", func(t *testing.T) {
		// Validate a queue with no reservation.
		candidate := state.CloneVT()
		candidate.Ops = []*SOOperation{first}
		candidate.QueuedAccountNonces = nil
		if err := candidate.Validate(mockSharedObjectID); err != nil {
			t.Fatal(err)
		}
		if got := candidate.NextOperationLink(peers[0].GetPeerID().String()).Nonce; got != 2 {
			t.Fatalf("pending operation requires next nonce 2, got %d", got)
		}
	})
}
