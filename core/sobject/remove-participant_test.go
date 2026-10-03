package sobject

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
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
// key and the checkpoint, with proofs it can verify, when the original
// creator is removed.
func TestRemoveSOParticipantPreservesCreatorContent(t *testing.T) {
	// The creator signs the genesis config, checkpoint and grants of two owners.
	ctx := t.Context()
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	initial, genesis := newTestSOState(t, peers,
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_OWNER,
	)
	key, err := initial.CurrentKeyEpoch().FindGrant(peers[1].GetPeerID().String()).DecryptInnerData(keys[1], mockSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	host, state := newLeaveTestHost(t, initial.GetConfig(), genesis)
	*state = initial.CloneVT()

	// The second owner removes the creator.
	if _, err := RemoveSOParticipant(ctx, host, peers[0].GetPeerID().String(), keys[1], nil); err != nil {
		t.Fatal(err)
	}

	// The same checkpoint and key remain, with proofs the second owner signed.
	next := *state
	if err := next.Validate(mockSharedObjectID); err != nil {
		t.Fatalf("creator removal invalidated the shared state: %v", err)
	}
	if err := next.ValidateAuthority(mockSharedObjectID); err != nil {
		t.Fatalf("remaining owner cannot verify the proofs: %v", err)
	}
	if !bytes.Equal(next.GetCheckpoint().Hash(), initial.GetCheckpoint().Hash()) {
		t.Fatal("creator removal changed the checkpoint")
	}
	grants := next.CurrentKeyEpoch().GetGrants()
	if len(grants) != 1 {
		t.Fatal("creator grant survived removal")
	}
	inner, err := grants[0].DecryptInnerData(keys[1], mockSharedObjectID)
	if err != nil || !inner.EqualVT(key) {
		t.Fatalf("remaining owner lost the content key: %v", err)
	}
}

// newRemovedSignerFixture builds a host whose current key epoch was granted by
// a second owner, plus an operation from a writer. Participants are owner,
// second owner, writer and a remaining writer.
func newRemovedSignerFixture(t *testing.T) (*SOHost, **SOState, *SharedObjectConfig, []crypto.PrivKey, []peer.Peer) {
	// Use two owners and two writers.
	t.Helper()
	peers := createMockPeers(t, 4)
	keys := mustPrivKeys(t, peers)
	initial, genesis := newTestSOState(t, peers,
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_OWNER,
	)

	// The second owner rotates the key.
	_, epoch, err := RotateTransformKey(keys[1], mockSharedObjectID, initial.GetConfig().GetParticipants(), initial.CurrentKeyEpoch().GetEpoch(), initial.GetConfig().GetConfigChainSeqno())
	if err != nil {
		t.Fatal(err)
	}
	initial.SetKeyEpoch(epoch)

	// The first writer writes.
	writeTestOp(t, initial, keys[2], "op")
	if err := initial.Validate(mockSharedObjectID); err != nil {
		t.Fatal(err)
	}
	host, state := newLeaveTestHost(t, initial.GetConfig(), genesis)
	*state = initial
	return host, state, initial.GetConfig(), keys, peers
}

// TestRemoveSOParticipantsReplacesRemovedSigners verifies that removing an
// owner and a writer leaves every remaining grant verifiable under the next
// configuration, and keeps the writer's operation for replay to skip.
func TestRemoveSOParticipantsReplacesRemovedSigners(t *testing.T) {
	// Remove the second and third peers as the first owner.
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
	if err := next.ValidateAuthority(mockSharedObjectID); err != nil {
		t.Fatalf("removal left proofs of a removed signer: %v", err)
	}
	if len(next.CurrentKeyEpoch().GetGrants()) != 2 || len(next.GetOps()) != 1 {
		t.Fatalf("removal left %d grants and %d ops", len(next.CurrentKeyEpoch().GetGrants()), len(next.GetOps()))
	}
}

// TestLeaveSOParticipantsReplacesDepartedSigner verifies that a departing
// owner's grants do not invalidate the remaining state.
func TestLeaveSOParticipantsReplacesDepartedSigner(t *testing.T) {
	// The second peer leaves.
	host, state, config, keys, _ := newRemovedSignerFixture(t)
	request, err := BuildSOLeaveRequest(mockSharedObjectID, config.GetConfigChainHash(), keys[1])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LeaveSOParticipants(t.Context(), host, keys[0], request); err != nil {
		t.Fatal(err)
	}
	next := *state
	if err := next.ValidateAuthority(mockSharedObjectID); err != nil {
		t.Fatalf("departure left proofs of the departed owner: %v", err)
	}
	if len(next.CurrentKeyEpoch().GetGrants()) != 3 || len(next.GetOps()) != 1 {
		t.Fatalf("departure left %d grants and %d ops", len(next.CurrentKeyEpoch().GetGrants()), len(next.GetOps()))
	}
}
