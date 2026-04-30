package sobject

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	blockenc_conf "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// Mock shared object ID for testing
const mockSharedObjectID = "test_object"

// createMockPeers creates the specified number of mock peers for testing
func createMockPeers(t *testing.T, count uint64) []peer.Peer {
	peers := make([]peer.Peer, count)
	for i := range count {
		p, err := peer.NewPeer(nil)
		if err != nil {
			t.Fatalf("Failed to create peer%d: %v", i+1, err)
		}
		peers[i] = p
	}
	return peers
}

func mustMarshalVT[T interface{ MarshalVT() ([]byte, error) }](
	t *testing.T,
	msg T,
) []byte {
	t.Helper()
	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

// createMockSOState creates a mock SOState for testing
func createMockSOState(peers []peer.Peer, roles []SOParticipantRole) *SOState {
	participants := make([]*SOParticipantConfig, len(peers))

	for i, p := range peers {
		peerIDStr := p.GetPeerID().String()
		role := SOParticipantRole_SOParticipantRole_VALIDATOR
		if i < len(roles) {
			role = roles[i]
		}
		participants[i] = &SOParticipantConfig{
			PeerId: peerIDStr,
			Role:   role,
		}
	}

	initialInner := &SORootInner{
		Seqno:     1,
		StateData: []byte("initial state"),
	}
	initialInnerData, _ := initialInner.MarshalVT()

	return &SOState{
		Config: &SharedObjectConfig{
			Participants: participants,
		},
		Root: &SORoot{
			Inner:      initialInnerData,
			InnerSeqno: 1,
		},
	}
}

// createMockSORoot creates a mock SORoot for testing
func createMockSORoot(t *testing.T, seqno uint64, signers ...peer.Peer) *SORoot {
	innerRoot := &SORootInner{
		Seqno:     seqno,
		StateData: []byte("new state"),
	}
	innerData, err := innerRoot.MarshalVT()
	if err != nil {
		t.Fatalf("Failed to marshal inner root: %v", err)
	}

	// Create account nonces for all signers
	accountNonces := make([]*SOAccountNonce, len(signers))
	for i, signer := range signers {
		accountNonces[i] = &SOAccountNonce{
			PeerId: signer.GetPeerID().String(),
			Nonce:  0,
		}
	}
	// Sort account nonces by peer ID
	slices.SortFunc(accountNonces, func(a, b *SOAccountNonce) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	})

	root := &SORoot{
		Inner:         innerData,
		InnerSeqno:    seqno,
		AccountNonces: accountNonces,
	}

	// Sign with each signer
	for _, signer := range signers {
		priv, err := signer.GetPrivKey(context.Background())
		if err != nil {
			t.Fatalf("Failed to get private key: %v", err)
		}

		if err := root.SignInnerData(priv, mockSharedObjectID, seqno, hash.HashType_HashType_BLAKE3); err != nil {
			t.Fatalf("Failed to sign root: %v", err)
		}
	}

	return root
}

// createMockSOOperation creates a mock SOOperation for testing
func createMockSOOperation(t *testing.T, privKey crypto.PrivKey, nonce uint64) (*SOOperation, *SOOperationInner) {
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		t.Fatalf("Failed to get peer ID from private key: %v", err)
	}
	peerIDStr := peerID.String()

	localID := NewSOOperationLocalID()
	inner := &SOOperationInner{
		PeerId:  peerIDStr,
		LocalId: localID,
		Nonce:   nonce,
		OpData:  []byte("test operation"),
	}
	innerData, err := inner.MarshalVT()
	if err != nil {
		t.Fatalf("Failed to marshal operation inner: %v", err)
	}

	encContext := BuildSOOperationSignatureContext(mockSharedObjectID, peerIDStr, nonce, localID)
	sig, err := peer.NewSignature(encContext, privKey, hash.HashType_HashType_BLAKE3, innerData, true)
	if err != nil {
		t.Fatalf("Failed to create operation signature: %v", err)
	}

	return &SOOperation{
		Inner:     innerData,
		Signature: sig,
	}, inner
}

func TestUpdateRootState(t *testing.T) {
	// Create test peers
	peers := createMockPeers(t, 3)
	peer1, peer2, peer3 := peers[0], peers[1], peers[2]

	// Get peer ID strings
	peer1IDStr := peer1.GetPeerID().String()
	peer2IDStr := peer2.GetPeerID().String()
	peer3IDStr := peer3.GetPeerID().String()

	t.Run("Valid update", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		nextRoot := createMockSORoot(t, 2, peer1)

		err := state.UpdateRootState(mockSharedObjectID, nextRoot, peer1IDStr, nil, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !state.Root.EqualVT(nextRoot) {
			t.Fatalf("Expected state.Root to be updated")
		}
		if len(state.Ops) != 0 {
			t.Fatalf("Expected state.Ops to be empty, got %d operations", len(state.Ops))
		}
		if len(state.OpRejections) != 0 {
			t.Fatalf("Expected state.OpRejections to be empty, got %d rejections", len(state.OpRejections))
		}
	})

	// Parameterized rejection cases: each builds a single invalid next-root and
	// calls UpdateRootState with no rejections/no acceptances. Cases that need
	// extra setup (operations queued, custom roles) stay as their own runs.
	t.Run("Rejects invalid next-root", func(t *testing.T) {
		type rejectionCase struct {
			name      string
			buildRoot func(t *testing.T) *SORoot
			validator string
			wantIs    error  // when non-nil, errors.Is must match.
			wantSub   string // when non-empty, error message must contain.
		}

		mustResign := func(t *testing.T, nextRoot *SORoot) {
			t.Helper()
			nextRoot.ValidatorSignatures = nil
			peer1PrivKey, _ := peer1.GetPrivKey(context.Background())
			if err := nextRoot.SignInnerData(peer1PrivKey, mockSharedObjectID, 2, hash.HashType_HashType_BLAKE3); err != nil {
				t.Fatalf("Failed to sign root: %v", err)
			}
		}

		cases := []rejectionCase{
			{
				name:      "Enforce wrong validator",
				buildRoot: func(t *testing.T) *SORoot { return createMockSORoot(t, 2, peer1) },
				validator: peer2IDStr,
				wantIs:    ErrInvalidValidator,
			},
			{
				name:      "Invalid seqno",
				buildRoot: func(t *testing.T) *SORoot { return createMockSORoot(t, 3, peer1) },
				validator: peer1IDStr,
				wantIs:    ErrInvalidSeqno,
			},
			{
				name: "Invalid account nonces order",
				buildRoot: func(t *testing.T) *SORoot {
					nr := createMockSORoot(t, 2, peer1, peer2)
					nr.AccountNonces[0], nr.AccountNonces[1] = nr.AccountNonces[1], nr.AccountNonces[0]
					mustResign(t, nr)
					return nr
				},
				validator: peer1IDStr,
				wantSub:   "account nonces not sorted",
			},
			{
				name: "Empty peer ID in account nonces",
				buildRoot: func(t *testing.T) *SORoot {
					nr := createMockSORoot(t, 2, peer1, peer2)
					nr.AccountNonces = append(nr.AccountNonces, &SOAccountNonce{
						PeerId: "",
						Nonce:  0,
					})
					return nr
				},
				validator: peer1IDStr,
				wantSub:   "peer id cannot be empty",
			},
			{
				name:      "Lower sequence number",
				buildRoot: func(t *testing.T) *SORoot { return createMockSORoot(t, 1, peer1) },
				validator: peer1IDStr,
				wantIs:    ErrInvalidSeqno,
			},
			{
				name: "Invalid inner state",
				buildRoot: func(t *testing.T) *SORoot {
					nr := createMockSORoot(t, 2, peer1)
					nr.Inner = nil
					return nr
				},
				validator: peer1IDStr,
				wantSub:   "",
			},
			{
				name: "Mismatched inner sequence number",
				buildRoot: func(t *testing.T) *SORoot {
					nr := createMockSORoot(t, 2, peer1)
					nr.InnerSeqno = 3
					return nr
				},
				validator: peer1IDStr,
				wantSub:   "",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				state := createMockSOState(peers, nil)
				nextRoot := tc.buildRoot(t)

				err := state.UpdateRootState(mockSharedObjectID, nextRoot, tc.validator, nil, nil)
				if tc.wantIs != nil {
					if !errors.Is(err, tc.wantIs) {
						t.Fatalf("Expected error %v, got %v", tc.wantIs, err)
					}
					return
				}
				if err == nil {
					t.Fatalf("Expected an error, got nil")
				}
				if tc.wantSub != "" && !strings.Contains(err.Error(), tc.wantSub) {
					t.Fatalf("Expected error containing %q, got: %v", tc.wantSub, err)
				}
			})
		}
	})

	t.Run("Multiple valid signatures", func(t *testing.T) {
		state := createMockSOState(peers, nil) // Default to all validators
		nextRoot := createMockSORoot(t, 2, peer1, peer2)

		err := state.UpdateRootState(mockSharedObjectID, nextRoot, peer1IDStr, nil, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Test with the other validator
		state = createMockSOState(peers, nil)
		err = state.UpdateRootState(mockSharedObjectID, nextRoot, peer2IDStr, nil, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	})

	t.Run("Invalid operation signature", func(t *testing.T) {
		state := createMockSOState(peers, []SOParticipantRole{SOParticipantRole_SOParticipantRole_VALIDATOR, SOParticipantRole_SOParticipantRole_WRITER})
		writerPrivKey, _ := peers[1].GetPrivKey(context.Background())

		op, err := BuildSOOperation(mockSharedObjectID, writerPrivKey, []byte("test operation"), 1, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}

		// Corrupt the signature
		op.Signature.SigData[0] ^= 0xFF

		err = state.QueueOperation(mockSharedObjectID, op)
		if err == nil {
			t.Fatal("Expected an error for signature invalid, got nil")
		}
		if !strings.Contains(err.Error(), "signature invalid") {
			t.Fatalf("Expected error to contain 'signature invalid', got: %v", err)
		}
	})

	t.Run("Non-validator signature", func(t *testing.T) {
		state := createMockSOState(peers, nil)

		// Create a non-validator peer
		nonValidatorPeer, _ := peer.NewPeer(nil)
		nonValidatorPeerIDStr := nonValidatorPeer.GetPeerID().String()

		// Create a root signed by the non-validator
		nextRoot := createMockSORoot(t, 2, nonValidatorPeer)

		err := state.UpdateRootState(mockSharedObjectID, nextRoot, nonValidatorPeerIDStr, nil, nil)
		if err == nil {
			t.Fatal("Expected an error for non-validator signature, got nil")
		}
	})

	t.Run("Empty validator signatures", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		nextRoot := createMockSORoot(t, 2)
		nextRoot.ValidatorSignatures = nil

		err := state.UpdateRootState(mockSharedObjectID, nextRoot, "", nil, nil)
		if err == nil {
			t.Fatal("Expected an error for empty validator signatures, got nil")
		}
	})

	t.Run("Identical root state", func(t *testing.T) {
		// Kept as its own run because the next-root is constructed by cloning
		// state.Root rather than via createMockSORoot.
		state := createMockSOState(peers, nil)
		nextRoot := state.Root.CloneVT()

		err := state.UpdateRootState(mockSharedObjectID, nextRoot, peer1IDStr, nil, nil)
		if !errors.Is(err, ErrInvalidSeqno) {
			t.Fatalf("Expected error %v, got %v", ErrInvalidSeqno, err)
		}
	})

	t.Run("Accept new operation", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		ctx := context.Background()
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)
		op, _ := createMockSOOperation(t, peer1PrivKey, 1)
		state.Ops = append(state.Ops, op)

		nextInner := &SORootInner{
			Seqno:     2,
			StateData: []byte("new state with applied operation"),
		}
		nextInnerData, err := nextInner.MarshalVT()
		if err != nil {
			t.Fatalf("Failed to marshal next inner: %v", err)
		}

		nextRoot := &SORoot{
			Inner: nextInnerData,
			AccountNonces: []*SOAccountNonce{
				{PeerId: peer1IDStr, Nonce: 1},
				{PeerId: peer2IDStr, Nonce: 0},
				{PeerId: peer3IDStr, Nonce: 0},
			},
			InnerSeqno: 2,
		}

		// sort account nonces
		slices.SortFunc(nextRoot.AccountNonces, func(a, b *SOAccountNonce) int {
			return strings.Compare(a.GetPeerId(), b.GetPeerId())
		})

		// Sign the next root with both peer1 and peer2
		for _, signer := range []peer.Peer{peer1, peer2} {
			privKey, err := signer.GetPrivKey(ctx)
			if err != nil {
				t.Fatalf("Failed to get private key: %v", err)
			}
			err = nextRoot.SignInnerData(privKey, mockSharedObjectID, 2, hash.HashType_HashType_BLAKE3)
			if err != nil {
				t.Fatalf("Failed to sign next root: %v", err)
			}
		}

		err = state.UpdateRootState(mockSharedObjectID, nextRoot, peer1IDStr, nil, []*SOOperation{op})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !state.Root.EqualVT(nextRoot) {
			t.Fatalf("Expected state.Root to be updated")
		}
		if len(state.Ops) != 0 {
			t.Fatalf("Expected operations to be cleared, got %d operations", len(state.Ops))
		}
		if len(state.OpRejections) != 0 {
			t.Fatalf("Expected no rejections, got %d rejections", len(state.OpRejections))
		}
	})

	t.Run("Reject operation", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		ctx := context.Background()
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)
		peer2PrivKey, _ := peer2.GetPrivKey(ctx)
		op, opInner := createMockSOOperation(t, peer1PrivKey, 1)
		state.Ops = append(state.Ops, op)

		rejection, err := BuildSOOperationRejection(
			peer2PrivKey,
			mockSharedObjectID,
			peer1.GetPeerID(),
			1,
			opInner.GetLocalId(),
			nil, // No error details
		)
		if err != nil {
			t.Fatalf("Failed to build operation rejection: %v", err)
		}

		nextRoot := createMockSORoot(t, 2, peer1, peer2)

		err = state.UpdateRootState(mockSharedObjectID, nextRoot, peer1IDStr, []*SOOperationRejection{rejection}, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(state.Ops) != 0 {
			t.Fatalf("Expected operations to be cleared, got %d operations", len(state.Ops))
		}
		if len(state.OpRejections) != 1 {
			t.Fatalf("Expected 1 operation rejection, got %d", len(state.OpRejections))
		}
		if state.OpRejections[0].GetPeerId() != peer1IDStr {
			t.Fatalf("Expected rejection for peer %s, got %s", peer1IDStr, state.OpRejections[0].GetPeerId())
		}
		if len(state.OpRejections[0].GetRejections()) != 1 {
			t.Fatalf("Expected 1 rejection for peer, got %d", len(state.OpRejections[0].GetRejections()))
		}
	})

	t.Run("Reject non-existent operation", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		ctx := context.Background()
		peer2PrivKey, _ := peer2.GetPrivKey(ctx)
		rejection, err := BuildSOOperationRejection(
			peer2PrivKey,
			mockSharedObjectID,
			peer1.GetPeerID(),
			1,                       // random nonce
			NewSOOperationLocalID(), // random id
			nil,                     // No error details
		)
		if err != nil {
			t.Fatalf("Failed to build operation rejection: %v", err)
		}
		nextRoot := createMockSORoot(t, 2, peer1, peer2)

		err = state.UpdateRootState(mockSharedObjectID, nextRoot, peer1IDStr, []*SOOperationRejection{rejection}, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(state.OpRejections) != 1 {
			t.Fatalf("Expected 1 operation rejection, got %d", len(state.OpRejections))
		}
		if state.OpRejections[0].GetPeerId() != peer1IDStr {
			t.Fatalf("Expected rejection for peer %s, got %s", peer1IDStr, state.OpRejections[0].GetPeerId())
		}
		if len(state.OpRejections[0].GetRejections()) != 1 {
			t.Fatalf("Expected 1 rejection for peer, got %d", len(state.OpRejections[0].GetRejections()))
		}
	})

	t.Run("Decode error details", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		ctx := context.Background()
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)
		peer2PrivKey, _ := peer2.GetPrivKey(ctx)
		op, inner := createMockSOOperation(t, peer1PrivKey, 1)
		state.Ops = append(state.Ops, op)

		errorDetails := &SOOperationRejectionErrorDetails{
			ErrorMsg: "Test error message",
		}

		rejection, err := BuildSOOperationRejection(
			peer2PrivKey,
			mockSharedObjectID,
			peer1.GetPeerID(),
			1,
			inner.GetLocalId(),
			errorDetails,
		)
		if err != nil {
			t.Fatalf("Failed to build operation rejection: %v", err)
		}

		nextRoot := createMockSORoot(t, 2, peer1, peer2)

		err = state.UpdateRootState(mockSharedObjectID, nextRoot, peer1IDStr, []*SOOperationRejection{rejection}, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(state.OpRejections) != 1 {
			t.Fatalf("Expected 1 operation rejection, got %d", len(state.OpRejections))
		}

		// Decode the error details
		rejectionInner := &SOOperationRejectionInner{}
		if err := rejectionInner.UnmarshalVT(state.OpRejections[0].Rejections[0].GetInner()); err != nil {
			t.Fatalf("Failed to unmarshal rejection inner: %v", err)
		}
		decodedErrorDetails, err := rejectionInner.DecodeErrorDetails(peer1PrivKey, mockSharedObjectID, peer2.GetPeerID())
		if err != nil {
			t.Fatalf("Failed to decode error details: %v", err)
		}

		if decodedErrorDetails.GetErrorMsg() != errorDetails.GetErrorMsg() {
			t.Fatalf("Expected error message '%s', got '%s'", errorDetails.ErrorMsg, decodedErrorDetails.ErrorMsg)
		}

		// Test with incorrect private key
		wrongPrivKey, _, err := crypto.GenerateEd25519Key(nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		_, err = rejectionInner.DecodeErrorDetails(wrongPrivKey, mockSharedObjectID, peer2.GetPeerID())
		if err == nil {
			t.Fatal("Expected an error when decoding with incorrect private key, got nil")
		}
	})

	// Parameterized: combinations of accept and reject indices for a fixed
	// queue of three operations. Covers the previous "Accept and reject
	// multiple operations" and "Reject all operations" cases.
	t.Run("Accept and reject combinations", func(t *testing.T) {
		ctx := context.Background()
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)
		peer2PrivKey, _ := peer2.GetPrivKey(ctx)

		cases := []struct {
			name        string
			rejectIdxs  []int
			acceptIdxs  []int
			wantRejects int
		}{
			{name: "accept middle, reject first and last", rejectIdxs: []int{0, 2}, acceptIdxs: []int{1}, wantRejects: 2},
			{name: "reject all three", rejectIdxs: []int{0, 1, 2}, acceptIdxs: nil, wantRejects: 3},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				state := createMockSOState(peers, nil)

				ops := make([]*SOOperation, 3)
				ids := make([]string, 3)
				for i := uint64(1); i <= 3; i++ {
					op, inner := createMockSOOperation(t, peer1PrivKey, i)
					state.Ops = append(state.Ops, op)
					ops[i-1] = op
					ids[i-1] = inner.GetLocalId()
				}

				rejections := make([]*SOOperationRejection, 0, len(tc.rejectIdxs))
				for _, idx := range tc.rejectIdxs {
					rej, err := BuildSOOperationRejection(peer2PrivKey, mockSharedObjectID, peer1.GetPeerID(), uint64(idx+1), ids[idx], nil) //nolint:gosec
					if err != nil {
						t.Fatalf("BuildSOOperationRejection: %v", err)
					}
					rejections = append(rejections, rej)
				}

				accepted := make([]*SOOperation, 0, len(tc.acceptIdxs))
				for _, idx := range tc.acceptIdxs {
					accepted = append(accepted, ops[idx])
				}

				nextRoot := createMockSORoot(t, 2, peer1, peer2)

				err := state.UpdateRootState(
					mockSharedObjectID,
					nextRoot,
					peer1IDStr,
					rejections,
					accepted,
				)
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				if len(state.Ops) != 0 {
					t.Fatalf("Expected all operations to be processed, got %d remaining", len(state.Ops))
				}
				if len(state.OpRejections) != 1 {
					t.Fatalf("Expected 1 peer rejection, got %d", len(state.OpRejections))
				}
				if got := len(state.OpRejections[0].Rejections); got != tc.wantRejects {
					t.Fatalf("Expected %d rejections, got %d", tc.wantRejects, got)
				}
			})
		}
	})

	t.Run("Accept all operations", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		ctx := context.Background()
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)

		// Create and queue multiple operations
		for i := uint64(1); i <= 3; i++ {
			op, _ := createMockSOOperation(t, peer1PrivKey, i)
			state.Ops = append(state.Ops, op)
		}

		nextRoot := createMockSORoot(t, 2, peer1, peer2)

		// Update state, accepting all operations
		err := state.UpdateRootState(
			mockSharedObjectID,
			nextRoot,
			peer1IDStr,
			nil,
			state.Ops,
		)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(state.Ops) != 0 {
			t.Fatalf("Expected all operations to be processed, got %d remaining", len(state.Ops))
		}

		if len(state.OpRejections) != 0 {
			t.Fatalf("Expected no rejections, got %d", len(state.OpRejections))
		}
	})

	t.Run("Queue multiple operations", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		ctx := context.Background()
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)

		for i := uint64(1); i <= 3; i++ {
			op, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, fmt.Appendf(nil, "test operation %d", i), i, NewSOOperationLocalID())
			if err != nil {
				t.Fatalf("Unexpected error building operation %d: %v", i, err)
			}

			err = state.QueueOperation(mockSharedObjectID, op)
			if err != nil {
				t.Fatalf("Unexpected error queueing operation %d: %v", i, err)
			}
		}

		if len(state.Ops) != 3 {
			t.Fatalf("Expected 3 operations, got %d", len(state.Ops))
		}
	})

	t.Run("Queue operation with duplicate nonce", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		ctx := context.Background()
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)

		op1, _ := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation 1"), 1, NewSOOperationLocalID())
		err := state.QueueOperation(mockSharedObjectID, op1)
		if err != nil {
			t.Fatalf("Unexpected error queueing first operation: %v", err)
		}

		op2, _ := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation 2"), 1, NewSOOperationLocalID()) // Same nonce
		err = state.QueueOperation(mockSharedObjectID, op2)
		if err == nil {
			t.Fatal("Expected an error for duplicate nonce, got nil")
		}
	})
}

func TestSOGrantValidateSignatureAllowsSelfSignedParticipantGrant(t *testing.T) {
	p := createMockPeers(t, 1)[0]
	priv, err := p.GetPrivKey(context.Background())
	if err != nil {
		t.Fatalf("get peer privkey: %v", err)
	}
	pub, err := p.GetPeerID().ExtractPublicKey()
	if err != nil {
		t.Fatalf("extract peer pubkey: %v", err)
	}
	grant, err := EncryptSOGrant(
		priv,
		pub,
		mockSharedObjectID,
		&SOGrantInner{
			TransformConf: &block_transform.Config{
				Steps: []*block_transform.StepConfig{{
					Id: blockenc_conf.ConfigID,
					Config: mustMarshalVT(t, &blockenc_conf.Config{
						BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
						Key:      []byte("0123456789abcdef0123456789abcdef"),
					}),
				}},
			},
		},
	)
	if err != nil {
		t.Fatalf("EncryptSOGrant: %v", err)
	}
	err = grant.ValidateSignature(mockSharedObjectID, []*SOParticipantConfig{{
		PeerId: grant.GetPeerId(),
		Role:   SOParticipantRole_SOParticipantRole_READER,
	}})
	if err != nil {
		t.Fatalf("ValidateSignature: %v", err)
	}
}

func TestBuildSOOperation(t *testing.T) {
	ctx := context.Background()
	peers := createMockPeers(t, 1)
	peer1 := peers[0]

	t.Run("Valid operation", func(t *testing.T) {
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)
		op, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation"), 1, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if op == nil {
			t.Fatal("Expected non-nil operation")
		}
		if len(op.GetInner()) == 0 {
			t.Fatal("Expected non-empty inner data")
		}
		if op.GetSignature() == nil {
			t.Fatal("Expected non-nil signature")
		}
	})

	t.Run("Empty operation data", func(t *testing.T) {
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)
		_, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte{}, 1, NewSOOperationLocalID())
		if err == nil {
			t.Fatal("Expected an error for empty operation data, got nil")
		}
	})

	t.Run("Zero nonce", func(t *testing.T) {
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)
		_, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation"), 0, NewSOOperationLocalID())
		if err == nil {
			t.Fatal("Expected an error for zero nonce, got nil")
		}
	})

	t.Run("Large operation data", func(t *testing.T) {
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)
		largeData := make([]byte, MaxInnerDataSize+1)
		_, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, largeData, 1, NewSOOperationLocalID())
		if err == nil {
			t.Fatal("Expected an error for large operation data, got nil")
		}
	})
}

func TestNonceTracking(t *testing.T) {
	peers := createMockPeers(t, 3)
	peer1, peer2 := peers[0], peers[1]
	ctx := context.Background()

	t.Run("Initial nonce state", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		nextNonce := state.GetNextAccountNonce(peer1.GetPeerID().String())
		if nextNonce != 1 {
			t.Fatalf("Expected initial nonce to be 1, got %d", nextNonce)
		}
	})

	t.Run("Queued nonce tracking", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)

		// Queue first operation
		op1, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation 1"), 1, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}
		if err := state.QueueOperation(mockSharedObjectID, op1); err != nil {
			t.Fatalf("Unexpected error queueing operation: %v", err)
		}

		// Check queued nonce
		var found bool
		for _, nonce := range state.GetQueuedAccountNonces() {
			if nonce.GetPeerId() == peer1.GetPeerID().String() {
				if nonce.GetNonce() != 1 {
					t.Fatalf("Expected queued nonce to be 1, got %d", nonce.GetNonce())
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatal("Expected to find queued nonce for peer1")
		}

		// Next nonce should be 2
		nextNonce := state.GetNextAccountNonce(peer1.GetPeerID().String())
		if nextNonce != 2 {
			t.Fatalf("Expected next nonce to be 2, got %d", nextNonce)
		}
	})

	t.Run("Multiple peer nonce tracking", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)
		peer2PrivKey, _ := peer2.GetPrivKey(ctx)

		// Queue operations for both peers
		op1, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation 1"), 1, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}
		if err := state.QueueOperation(mockSharedObjectID, op1); err != nil {
			t.Fatalf("Unexpected error queueing operation: %v", err)
		}

		op2, err := BuildSOOperation(mockSharedObjectID, peer2PrivKey, []byte("test operation 2"), 1, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}
		if err := state.QueueOperation(mockSharedObjectID, op2); err != nil {
			t.Fatalf("Unexpected error queueing operation: %v", err)
		}

		// Verify queued nonces are sorted by peer ID
		queuedNonces := state.GetQueuedAccountNonces()
		if len(queuedNonces) != 2 {
			t.Fatalf("Expected 2 queued nonces, got %d", len(queuedNonces))
		}
		if !slices.IsSortedFunc(queuedNonces, func(a, b *SOAccountNonce) int {
			return strings.Compare(a.GetPeerId(), b.GetPeerId())
		}) {
			t.Fatal("Queued nonces not sorted by peer ID")
		}
	})

	t.Run("Nonce cleanup after root update", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)

		// Queue operation
		op, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation"), 1, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}
		if err := state.QueueOperation(mockSharedObjectID, op); err != nil {
			t.Fatalf("Unexpected error queueing operation: %v", err)
		}

		// Create new root state with higher nonce
		nextRoot := createMockSORoot(t, 2, peer1)
		nextRoot.AccountNonces = []*SOAccountNonce{{
			PeerId: peer1.GetPeerID().String(),
			Nonce:  1,
		}}
		// Re-sign after updating account nonces
		nextRoot.ValidatorSignatures = nil
		if err := nextRoot.SignInnerData(peer1PrivKey, mockSharedObjectID, 2, hash.HashType_HashType_BLAKE3); err != nil {
			t.Fatalf("Failed to sign root: %v", err)
		}

		// Update root state
		err = state.UpdateRootState(mockSharedObjectID, nextRoot, peer1.GetPeerID().String(), nil, []*SOOperation{op})
		if err != nil {
			t.Fatalf("Unexpected error updating root state: %v", err)
		}

		// Verify queued nonce was cleaned up
		for _, nonce := range state.GetQueuedAccountNonces() {
			if nonce.GetPeerId() == peer1.GetPeerID().String() && nonce.GetNonce() <= 1 {
				t.Fatal("Expected queued nonce to be cleaned up")
			}
		}
	})

	t.Run("Nonce skipping with rejected operations", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)
		peer2PrivKey, _ := peer2.GetPrivKey(ctx)

		// Queue two operations
		op1, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation 1"), 1, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}
		if err := state.QueueOperation(mockSharedObjectID, op1); err != nil {
			t.Fatalf("Unexpected error queueing operation: %v", err)
		}

		op2, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation 2"), 2, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}
		if err := state.QueueOperation(mockSharedObjectID, op2); err != nil {
			t.Fatalf("Unexpected error queueing operation: %v", err)
		}

		// Create rejection for first operation
		inner1, err := op1.UnmarshalInner()
		if err != nil {
			t.Fatalf("Failed to unmarshal operation: %v", err)
		}
		rejection, err := BuildSOOperationRejection(
			peer2PrivKey,
			mockSharedObjectID,
			peer1.GetPeerID(),
			inner1.GetNonce(),
			inner1.GetLocalId(),
			nil,
		)
		if err != nil {
			t.Fatalf("Failed to build rejection: %v", err)
		}

		// Create new root state accepting second operation
		nextRoot := createMockSORoot(t, 2, peer1)
		nextRoot.AccountNonces = []*SOAccountNonce{{
			PeerId: peer1.GetPeerID().String(),
			Nonce:  2,
		}}
		// Re-sign after updating account nonces
		nextRoot.ValidatorSignatures = nil
		if err := nextRoot.SignInnerData(peer1PrivKey, mockSharedObjectID, 2, hash.HashType_HashType_BLAKE3); err != nil {
			t.Fatalf("Failed to sign root: %v", err)
		}

		// Update root state
		err = state.UpdateRootState(
			mockSharedObjectID,
			nextRoot,
			peer1.GetPeerID().String(),
			[]*SOOperationRejection{rejection},
			[]*SOOperation{op2},
		)
		if err != nil {
			t.Fatalf("Unexpected error updating root state: %v", err)
		}

		// Verify nonce state
		nextNonce := state.GetNextAccountNonce(peer1.GetPeerID().String())
		if nextNonce != 3 {
			t.Fatalf("Expected next nonce to be 3, got %d", nextNonce)
		}
	})

	t.Run("Invalid nonce order", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)

		// Try to queue operation with nonce 2 first
		op1, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation 1"), 2, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}
		err = state.QueueOperation(mockSharedObjectID, op1)
		if err == nil {
			t.Fatal("Expected error queueing operation with invalid nonce")
		}
		if !errors.Is(err, ErrInvalidNonce) {
			t.Fatalf("Expected ErrInvalidNonce, got %v", err)
		}
	})
}

func TestQueueOperation(t *testing.T) {
	ctx := context.Background()

	// Create test peers
	peers := createMockPeers(t, 2)
	peer1, peer2 := peers[0], peers[1]

	t.Run("Valid operation", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)

		op, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation"), 1, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}

		err = state.QueueOperation(mockSharedObjectID, op)
		if err != nil {
			t.Fatalf("Unexpected error queueing operation: %v", err)
		}
		if len(state.Ops) != 1 {
			t.Fatalf("Expected 1 operation, got %d", len(state.Ops))
		}
	})

	t.Run("Invalid nonce", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)

		op, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation"), 2, NewSOOperationLocalID()) // Should be 1
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}

		err = state.QueueOperation(mockSharedObjectID, op)
		if err == nil {
			t.Fatal("Expected an error for invalid nonce, got nil")
		}
		if !errors.Is(err, ErrInvalidNonce) {
			t.Fatalf("Expected error %v, got %v", ErrInvalidNonce, err)
		}
	})

	t.Run("Unauthorized peer", func(t *testing.T) {
		state := createMockSOState(peers, []SOParticipantRole{
			SOParticipantRole_SOParticipantRole_VALIDATOR,
			SOParticipantRole_SOParticipantRole_WRITER,
		})
		unauthorizedPeer := createMockPeers(t, 1)[0]
		unauthorizedPrivKey, _ := unauthorizedPeer.GetPrivKey(ctx)

		op, err := BuildSOOperation(mockSharedObjectID, unauthorizedPrivKey, []byte("test operation"), 1, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}

		err = state.QueueOperation(mockSharedObjectID, op)
		if err == nil {
			t.Fatal("Expected an error for unauthorized peer, got nil")
		}
		if !errors.Is(err, ErrNotParticipant) {
			t.Fatalf("Expected error %v, got %v", ErrNotParticipant, err)
		}
	})

	t.Run("Valid writer submitting operation", func(t *testing.T) {
		state := createMockSOState(peers, []SOParticipantRole{SOParticipantRole_SOParticipantRole_VALIDATOR, SOParticipantRole_SOParticipantRole_WRITER})
		writerPrivKey, _ := peers[1].GetPrivKey(ctx)

		op, err := BuildSOOperation(mockSharedObjectID, writerPrivKey, []byte("test operation"), 1, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}

		err = state.QueueOperation(mockSharedObjectID, op)
		if err != nil {
			t.Fatalf("Unexpected error queueing operation: %v", err)
		}

		if len(state.Ops) != 1 {
			t.Fatalf("Expected 1 operation in queue, got %d", len(state.Ops))
		}
	})

	t.Run("Invalid operation signature", func(t *testing.T) {
		state := createMockSOState(peers, []SOParticipantRole{SOParticipantRole_SOParticipantRole_VALIDATOR, SOParticipantRole_SOParticipantRole_WRITER})
		writerPrivKey, _ := peers[1].GetPrivKey(ctx)

		op, err := BuildSOOperation(mockSharedObjectID, writerPrivKey, []byte("test operation"), 1, NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building operation: %v", err)
		}

		// Corrupt the signature
		op.Signature.SigData[0] ^= 0xFF

		err = state.QueueOperation(mockSharedObjectID, op)
		if err == nil {
			t.Fatal("Expected an error for signature invalid, got nil")
		}
		if !strings.Contains(err.Error(), "signature invalid") {
			t.Fatalf("Expected error to contain 'signature invalid', got: %v", err)
		}
	})

	t.Run("Maximum operations reached", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)

		for i := uint64(1); i <= MaxOperations; i++ {
			op, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation "+strconv.Itoa(int(i))), i, NewSOOperationLocalID())
			if err != nil {
				t.Fatalf("Unexpected error building operation %d: %v", i, err)
			}

			err = state.QueueOperation(mockSharedObjectID, op)
			if err != nil {
				t.Fatalf("Unexpected error on operation %d: %v", i, err)
			}
		}

		overflowOp, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation overflow"), uint64(MaxOperations+1), NewSOOperationLocalID())
		if err != nil {
			t.Fatalf("Unexpected error building overflow operation: %v", err)
		}

		err = state.QueueOperation(mockSharedObjectID, overflowOp)
		if err == nil {
			t.Fatal("Expected an error when maximum operations reached, got nil")
		}
		if !errors.Is(err, ErrMaxCountExceeded) {
			t.Fatalf("Expected error %v, got %v", ErrMaxCountExceeded, err)
		}
	})

	t.Run("Multiple operations with correct nonce", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)

		for i := uint64(1); i <= 3; i++ {
			op, err := BuildSOOperation(mockSharedObjectID, peer1PrivKey, fmt.Appendf(nil, "test operation %d", i), i, NewSOOperationLocalID())
			if err != nil {
				t.Fatalf("Unexpected error building operation %d: %v", i, err)
			}

			err = state.QueueOperation(mockSharedObjectID, op)
			if err != nil {
				t.Fatalf("Unexpected error queueing operation %d: %v", i, err)
			}
		}

		if len(state.Ops) != 3 {
			t.Fatalf("Expected 3 operations in queue, got %d", len(state.Ops))
		}
	})

	t.Run("Operation with non-incremental nonce", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)

		op1, _ := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation 1"), 1, NewSOOperationLocalID())
		err := state.QueueOperation(mockSharedObjectID, op1)
		if err != nil {
			t.Fatalf("Unexpected error queueing first operation: %v", err)
		}

		op2, _ := BuildSOOperation(mockSharedObjectID, peer1PrivKey, []byte("test operation 2"), 3, NewSOOperationLocalID()) // Should be 2
		err = state.QueueOperation(mockSharedObjectID, op2)
		if err == nil {
			t.Fatal("Expected an error for non-incremental nonce, got nil")
		}
		if !errors.Is(err, ErrInvalidNonce) {
			t.Fatalf("Expected error %v, got %v", ErrInvalidNonce, err)
		}
	})

	t.Run("Invalid inner data size", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		nextRoot := createMockSORoot(t, 2, peer1)

		// Set inner data to exceed max size
		nextRoot.Inner = make([]byte, MaxInnerDataSize+1)

		err := state.UpdateRootState(mockSharedObjectID, nextRoot, peer1.GetPeerID().String(), nil, nil)
		if err == nil {
			t.Fatal("Expected an error for inner data size exceeding maximum, got nil")
		}
		if !errors.Is(err, ErrMaxSizeExceeded) {
			t.Fatalf("Expected error %v, got %v", ErrMaxSizeExceeded, err)
		}
	})

	t.Run("Too many validator signatures", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		nextRoot := createMockSORoot(t, 2, peer1)

		// Add more signatures than allowed
		for range uint64(MaxValidatorSignatures + 1) {
			nextRoot.ValidatorSignatures = append(nextRoot.ValidatorSignatures, nextRoot.ValidatorSignatures[0])
		}

		err := state.UpdateRootState(mockSharedObjectID, nextRoot, peer1.GetPeerID().String(), nil, nil)
		if err == nil {
			t.Fatal("Expected an error for too many validator signatures, got nil")
		}
		if !errors.Is(err, ErrMaxCountExceeded) {
			t.Fatalf("Expected error %v, got %v", ErrMaxCountExceeded, err)
		}
	})

	t.Run("Duplicate validator signatures", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		nextRoot := createMockSORoot(t, 2, peer1)

		// Add duplicate signature
		nextRoot.ValidatorSignatures = append(nextRoot.ValidatorSignatures, nextRoot.ValidatorSignatures[0])

		err := state.UpdateRootState(mockSharedObjectID, nextRoot, peer1.GetPeerID().String(), nil, nil)
		if err == nil {
			t.Fatal("Expected an error for duplicate validator signatures, got nil")
		}
		if !strings.Contains(err.Error(), "duplicate validator signature") {
			t.Fatalf("Expected error about duplicate validator signature, got: %v", err)
		}
	})

	t.Run("Mixed accepted and rejected operations", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		ctx := context.Background()
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)
		peer2PrivKey, _ := peer2.GetPrivKey(ctx)

		// Create two operations
		op1, inner1 := createMockSOOperation(t, peer1PrivKey, 1)
		op2, _ := createMockSOOperation(t, peer1PrivKey, 2)
		state.Ops = append(state.Ops, op1, op2)

		// Create rejection for op1
		rejection, err := BuildSOOperationRejection(
			peer2PrivKey,
			mockSharedObjectID,
			peer1.GetPeerID(),
			inner1.GetNonce(),
			inner1.GetLocalId(),
			nil,
		)
		if err != nil {
			t.Fatalf("Failed to build operation rejection: %v", err)
		}

		nextRoot := createMockSORoot(t, 2, peer1, peer2)

		// Update state: reject op1, accept op2
		err = state.UpdateRootState(
			mockSharedObjectID,
			nextRoot,
			peer1.GetPeerID().String(),
			[]*SOOperationRejection{rejection},
			[]*SOOperation{op2},
		)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(state.Ops) != 0 {
			t.Fatalf("Expected all operations to be processed, got %d remaining", len(state.Ops))
		}

		if len(state.OpRejections) != 1 {
			t.Fatalf("Expected 1 rejection, got %d", len(state.OpRejections))
		}

		if len(state.OpRejections[0].GetRejections()) != 1 {
			t.Fatalf("Expected 1 rejection for peer, got %d", len(state.OpRejections[0].GetRejections()))
		}
	})

	t.Run("Invalid rejection signature", func(t *testing.T) {
		state := createMockSOState(peers, nil)
		ctx := context.Background()
		peer1PrivKey, _ := peer1.GetPrivKey(ctx)
		peer2PrivKey, _ := peer2.GetPrivKey(ctx)

		op, inner := createMockSOOperation(t, peer1PrivKey, 1)
		state.Ops = append(state.Ops, op)

		rejection, err := BuildSOOperationRejection(
			peer2PrivKey,
			mockSharedObjectID,
			peer1.GetPeerID(),
			inner.GetNonce(),
			inner.GetLocalId(),
			nil,
		)
		if err != nil {
			t.Fatalf("Failed to build operation rejection: %v", err)
		}

		// Corrupt the rejection signature
		rejection.Signature.SigData[0] ^= 0xFF

		nextRoot := createMockSORoot(t, 2, peer1, peer2)

		err = state.UpdateRootState(mockSharedObjectID, nextRoot, peer1.GetPeerID().String(), []*SOOperationRejection{rejection}, nil)
		if err == nil {
			t.Fatal("Expected an error for invalid rejection signature, got nil")
		}
		if !strings.Contains(err.Error(), "signature invalid") {
			t.Fatalf("Expected error about invalid signature, got: %v", err)
		}
	})
}
