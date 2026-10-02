package sobject

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestRemoveSOParticipantsAtomicallyRemovesAudience verifies that one owner
// transition removes a person's complete participant set.
func TestRemoveSOParticipantsAtomicallyRemovesAudience(t *testing.T) {
	// Use one owner and two writers.
	ctx := context.Background()
	peers := createMockPeers(t, 3)
	owner, err := peers[0].GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Record the owner genesis.
	initial := &SharedObjectConfig{Participants: []*SOParticipantConfig{
		{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: peers[1].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_WRITER},
		{PeerId: peers[2].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_WRITER},
	}}
	genesis, err := BuildSOConfigChange(mockSharedObjectID, initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := VerifyConfigChange(mockSharedObjectID, initial, genesis)
	if err != nil {
		t.Fatal(err)
	}
	host, state := newLeaveTestHost(t, checkpoint)

	// Remove both writers in one owner transition.
	removed, err := RemoveSOParticipants(ctx, host, []string{
		peers[1].GetPeerID().String(), peers[2].GetPeerID().String(),
	}, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed %d participants, want 2", len(removed))
	}

	// Only the owner remains, one config change later.
	current := (*state).GetConfig()
	if len(current.GetParticipants()) != 1 || current.GetParticipants()[0].GetPeerId() != peers[0].GetPeerID().String() {
		t.Fatalf("atomic removal left audience %v", current.GetParticipants())
	}
	if current.GetConfigChainSeqno() != checkpoint.GetConfigChainSeqno()+1 {
		t.Fatalf("atomic removal advanced configuration by %d changes", current.GetConfigChainSeqno()-checkpoint.GetConfigChainSeqno())
	}
}

// TestRemoveSOParticipantPreservesCreatorContent keeps a surviving owner's
// encrypted access and valid root proof when the original creator is removed.
func TestRemoveSOParticipantPreservesCreatorContent(t *testing.T) {
	// Use a creator and a second owner.
	ctx := t.Context()
	peers := createMockPeers(t, 2)
	creator, err := peers[0].GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := peers[1].GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Record the creator genesis.
	initial := &SharedObjectConfig{Participants: []*SOParticipantConfig{
		{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: peers[1].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
	}}
	genesis, err := BuildSOConfigChange(mockSharedObjectID, initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, creator, nil)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := VerifyConfigChange(mockSharedObjectID, initial, genesis)
	if err != nil {
		t.Fatal(err)
	}
	host, state := newLeaveTestHost(t, checkpoint)

	// Hold a root and grants signed by the creator.
	transform, grants, _, err := RotateTransformKey(creator, mockSharedObjectID, initial.GetParticipants(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	root := &SORoot{Inner: []byte("accepted encrypted content"), InnerSeqno: 1}
	if err := root.SignInnerData(creator, mockSharedObjectID, 1, hash.RecommendedHashType); err != nil {
		t.Fatal(err)
	}
	(*state).Root, (*state).RootGrants = root, grants

	// The second owner removes the creator.
	if _, err := RemoveSOParticipant(ctx, host, peers[0].GetPeerID().String(), owner, nil); err != nil {
		t.Fatal(err)
	}

	// The state stays valid with the same content and owner proof.
	next := *state
	if err := next.Validate(mockSharedObjectID); err != nil {
		t.Fatalf("creator removal invalidated the shared state: %v", err)
	}
	if !bytes.Equal(next.GetRoot().GetInner(), root.GetInner()) || next.GetRoot().GetInnerSeqno() != 1 {
		t.Fatal("creator removal changed accepted content")
	}
	if valid, err := next.GetRoot().ValidateSignatures(mockSharedObjectID, next.GetConfig().GetParticipants()); err != nil || valid != 1 {
		t.Fatalf("remaining owner cannot verify the root: %d, %v", valid, err)
	}

	// Only the remaining owner holds the content key.
	if len(next.GetRootGrants()) != 1 {
		t.Fatal("creator grant survived removal")
	}
	inner, err := next.GetRootGrants()[0].DecryptInnerData(owner, mockSharedObjectID)
	if err != nil || !inner.GetTransformConf().EqualVT(transform) {
		t.Fatalf("remaining owner lost the content key: %v", err)
	}
}

// newRemovedSignerFixture builds a host whose state holds a rejection and root
// grants signed by a validator, plus a queued operation from a writer.
// Participants are owner, validator, writer and a remaining writer.
func newRemovedSignerFixture(t *testing.T) (*SOHost, **SOState, *SharedObjectConfig, []crypto.PrivKey, []peer.Peer) {
	// Use an owner, a validator and two writers.
	t.Helper()
	ctx := t.Context()
	peers := createMockPeers(t, 4)
	keys := make([]crypto.PrivKey, len(peers))
	for i, p := range peers {
		key, err := p.GetPrivKey(ctx)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
	}

	// Record the owner genesis.
	initial := &SharedObjectConfig{Participants: []*SOParticipantConfig{
		{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: peers[1].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_VALIDATOR},
		{PeerId: peers[2].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_WRITER},
		{PeerId: peers[3].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_WRITER},
	}}
	genesis, err := BuildSOConfigChange(mockSharedObjectID, initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := VerifyConfigChange(mockSharedObjectID, initial, genesis)
	if err != nil {
		t.Fatal(err)
	}
	host, state := newLeaveTestHost(t, checkpoint, genesis)

	// The validator grants the root keys.
	_, grants, _, err := RotateTransformKey(keys[1], mockSharedObjectID, initial.GetParticipants(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	(*state).Root = createMockSORoot(t, 1, peers[0])
	(*state).RootGrants = grants

	// The first writer queues an operation.
	op, err := BuildSOOperation(mockSharedObjectID, keys[2], []byte("op"), linkAt(*state, keys[2], 1), NewSOOperationLocalID())
	if err != nil {
		t.Fatal(err)
	}
	if err := (*state).QueueOperation(mockSharedObjectID, op); err != nil {
		t.Fatal(err)
	}

	// The validator rejects the second writer's first operation.
	rejection, err := BuildSOOperationRejection(keys[1], mockSharedObjectID, peers[3].GetPeerID(), 1, NewSOOperationLocalID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	(*state).OpRejections = []*SOPeerOpRejections{{
		PeerId:     peers[3].GetPeerID().String(),
		Rejections: []*SOOperationRejection{rejection},
	}}
	if err := (*state).Validate(mockSharedObjectID); err != nil {
		t.Fatal(err)
	}
	return host, state, checkpoint, keys, peers
}

// TestRemoveSOParticipantsPrunesRemovedSigners verifies that removing a
// validator and a writer leaves no pending operation or rejection that the
// next configuration cannot verify.
func TestRemoveSOParticipantsPrunesRemovedSigners(t *testing.T) {
	host, state, _, keys, peers := newRemovedSignerFixture(t)
	if _, err := RemoveSOParticipants(t.Context(), host, []string{
		peers[1].GetPeerID().String(), peers[2].GetPeerID().String(),
	}, keys[0], nil); err != nil {
		t.Fatal(err)
	}
	next := *state
	if err := next.Validate(mockSharedObjectID); err != nil {
		t.Fatalf("removal left invalid state: %v", err)
	}
	if len(next.GetOps()) != 0 || len(next.GetOpRejections()) != 0 {
		t.Fatalf("removal retained %d ops and %d rejection groups", len(next.GetOps()), len(next.GetOpRejections()))
	}
}

// TestLeaveSOParticipantsPrunesDepartedSigner verifies that a departing
// validator's grants and rejections do not invalidate the remaining state.
func TestLeaveSOParticipantsPrunesDepartedSigner(t *testing.T) {
	host, state, checkpoint, keys, _ := newRemovedSignerFixture(t)
	request, err := BuildSOLeaveRequest(mockSharedObjectID, checkpoint.GetConfigChainHash(), keys[1])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LeaveSOParticipants(t.Context(), host, keys[0], request); err != nil {
		t.Fatal(err)
	}
	next := *state
	if err := next.Validate(mockSharedObjectID); err != nil {
		t.Fatalf("departure left invalid state: %v", err)
	}
	if len(next.GetRootGrants()) != 3 || len(next.GetOps()) != 1 || len(next.GetOpRejections()) != 0 {
		t.Fatalf("departure left %d grants, %d ops, %d rejection groups", len(next.GetRootGrants()), len(next.GetOps()), len(next.GetOpRejections()))
	}
}
