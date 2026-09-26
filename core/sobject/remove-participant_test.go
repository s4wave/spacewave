package sobject

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

// TestRemoveSOParticipantsAtomicallyRemovesAudience verifies that one owner
// transition removes a person's complete participant set.
func TestRemoveSOParticipantsAtomicallyRemovesAudience(t *testing.T) {
	ctx := context.Background()
	peers := createMockPeers(t, 3)
	owner, err := peers[0].GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	initial := &SharedObjectConfig{Participants: []*SOParticipantConfig{
		{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: peers[1].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_WRITER},
		{PeerId: peers[2].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_WRITER},
	}}
	genesis, err := BuildSOConfigChange(initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := VerifyConfigChange(initial, genesis)
	if err != nil {
		t.Fatal(err)
	}
	host, state := newLeaveTestHost(t, checkpoint)

	removed, err := RemoveSOParticipants(ctx, host, []string{
		peers[1].GetPeerID().String(), peers[2].GetPeerID().String(),
	}, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed %d participants, want 2", len(removed))
	}
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
	initial := &SharedObjectConfig{Participants: []*SOParticipantConfig{
		{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: peers[1].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
	}}
	genesis, err := BuildSOConfigChange(initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, creator, nil)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := VerifyConfigChange(initial, genesis)
	if err != nil {
		t.Fatal(err)
	}
	host, state := newLeaveTestHost(t, checkpoint)
	transform, grants, _, err := RotateTransformKey(creator, mockSharedObjectID, initial.GetParticipants(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	root := &SORoot{Inner: []byte("accepted encrypted content"), InnerSeqno: 1}
	if err := root.SignInnerData(creator, mockSharedObjectID, 1, hash.RecommendedHashType); err != nil {
		t.Fatal(err)
	}
	(*state).Root, (*state).RootGrants = root, grants
	if _, err := RemoveSOParticipant(ctx, host, peers[0].GetPeerID().String(), owner, nil); err != nil {
		t.Fatal(err)
	}
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
	if len(next.GetRootGrants()) != 1 {
		t.Fatal("creator grant survived removal")
	}
	inner, err := next.GetRootGrants()[0].DecryptInnerData(owner, mockSharedObjectID)
	if err != nil || !inner.GetTransformConf().EqualVT(transform) {
		t.Fatalf("remaining owner lost the content key: %v", err)
	}
}
