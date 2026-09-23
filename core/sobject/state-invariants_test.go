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
			state := base.CloneVT()
			state.Root.AccountNonces[0].Nonce = 5
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

	// Clearing a remotely observed rejection keeps its nonce reserved locally.
	t.Run("clear remote rejection", func(t *testing.T) {
		state := base.CloneVT()
		localID := NewSOOperationLocalID()
		rejection, err := BuildSOOperationRejection(priv, mockSharedObjectID,
			peers[0].GetPeerID(), 7, localID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := state.UpdateRootState(mockSharedObjectID, createMockSORoot(t, 2, peers[0]), "",
			[]*SOOperationRejection{rejection}, nil); err != nil {
			t.Fatal(err)
		}
		before := state.GetNextAccountNonce(peerID)
		clear, err := BuildSOClearOperationResult(mockSharedObjectID, priv, localID)
		if err != nil {
			t.Fatal(err)
		}
		if err := state.ClearOperationResult(mockSharedObjectID, clear); err != nil {
			t.Fatal(err)
		}
		if after := state.GetNextAccountNonce(peerID); after < before {
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
		op := signedLeanOperation(t, priv, peerID, 9, NewSOOperationLocalID())
		if err := state.UpdateRootState(mockSharedObjectID, createMockSORoot(t, 2, peers[0]), "",
			nil, []*SOOperation{op}); err != nil {
			t.Fatal(err)
		}
		if got := state.GetNextAccountNonce(peerID); got != 10 {
			t.Fatalf("accepted nonce 9 requires next nonce 10, got %d", got)
		}
	})

	// Exhaustion is sticky when the last consumed nonce came from a rejection.
	t.Run("exhaustion survives clearing", func(t *testing.T) {
		state := base.CloneVT()
		localID := NewSOOperationLocalID()
		rejection, err := BuildSOOperationRejection(priv, mockSharedObjectID,
			peers[0].GetPeerID(), math.MaxUint64, localID, nil)
		if err != nil {
			t.Fatal(err)
		}
		state.OpRejections = []*SOPeerOpRejections{{PeerId: peerID, Rejections: []*SOOperationRejection{rejection}}}
		clear, err := BuildSOClearOperationResult(mockSharedObjectID, priv, localID)
		if err != nil {
			t.Fatal(err)
		}
		if err := state.ClearOperationResult(mockSharedObjectID, clear); err != nil {
			t.Fatal(err)
		}
		if got := state.GetNextAccountNonce(peerID); got != 0 {
			t.Fatalf("exhausted account returned to usable nonce %d", got)
		}
		for _, nonce := range []uint64{0, 1, math.MaxUint64} {
			op := signedLeanOperation(t, priv, peerID, nonce, NewSOOperationLocalID())
			if err := state.QueueOperation(mockSharedObjectID, op); err == nil {
				t.Fatalf("exhausted account accepted nonce %d", nonce)
			}
		}
	})
}

// TestStateOperationSigner prevents an authorized writer from using another
// account's nonce and local-ID namespace.
func TestStateOperationSigner(t *testing.T) {
	peers := createMockPeers(t, 2)
	priv, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	state := createMockSOState(peers, nil)
	state.Root = createMockSORoot(t, 1, peers[0])
	op := signedLeanOperation(t, priv, peers[1].GetPeerID().String(), 1, NewSOOperationLocalID())
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
	localID := NewSOOperationLocalID()
	first, err := BuildSOOperation(mockSharedObjectID, priv, []byte("one"), 1, localID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildSOOperation(mockSharedObjectID, priv, []byte("two"), 2, localID)
	if err != nil {
		t.Fatal(err)
	}
	state.QueuedAccountNonces = []*SOAccountNonce{{PeerId: peers[0].GetPeerID().String(), Nonce: 2}}

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
		candidate := state.CloneVT()
		candidate.Ops = []*SOOperation{first}
		candidate.QueuedAccountNonces = nil
		if err := candidate.Validate(mockSharedObjectID); err != nil {
			t.Fatal(err)
		}
		if got := candidate.GetNextAccountNonce(peers[0].GetPeerID().String()); got != 2 {
			t.Fatalf("pending operation requires next nonce 2, got %d", got)
		}
	})
}
