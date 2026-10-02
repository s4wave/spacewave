package sobject

import (
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
)

// TestTransferSOOwnershipOwnerDeparture proves that a Space continues after its
// owner leaves: the default successor commits the carried departure, every
// remaining grant and the root verify under the new configuration, and a
// remaining participant can still submit operations.
func TestTransferSOOwnershipOwnerDeparture(t *testing.T) {
	// Load keys for an owner and two writers.
	ctx := t.Context()
	peers := createMockPeers(t, 3)
	keys := make([]crypto.PrivKey, len(peers))
	for i, candidate := range peers {
		key, err := candidate.GetPrivKey(ctx)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
	}

	// Commit the genesis configuration with the owner first.
	initial := createMockSOState(peers, []SOParticipantRole{
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_WRITER,
		SOParticipantRole_SOParticipantRole_WRITER,
	}).GetConfig()
	genesis, err := BuildSOConfigChange(mockSharedObjectID, initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := VerifyConfigChange(mockSharedObjectID, initial, genesis)
	if err != nil {
		t.Fatal(err)
	}
	host, state := newLeaveTestHost(t, checkpoint, genesis)

	// Grant the transform key and sign the root as the owner.
	_, grants, _, err := RotateTransformKey(keys[0], mockSharedObjectID, checkpoint.GetParticipants(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	root := createMockSORoot(t, 1, peers[0])
	if err := host.UpdateSOState(ctx, func(next *SOState) error {
		next.Root, next.RootGrants = root, grants
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// The owner alone cannot leave anyone dependent on its proofs.
	request, err := BuildSOLeaveRequest(mockSharedObjectID, checkpoint.GetConfigChainHash(), keys[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LeaveSOParticipants(ctx, host, keys[0], request); err == nil {
		t.Fatal("owner committed its own departure while participants remain")
	}

	// Promote the earliest writer; only the successor completes the departure.
	if _, err := TransferSOOwnership(ctx, host, keys[0], "", request); err != nil {
		t.Fatal(err)
	}
	if done, err := CompleteSOOwnershipTransfer(ctx, host, keys[0], ""); done || err != nil {
		t.Fatalf("departing owner completed its transfer: %v %v", done, err)
	}
	if done, err := CompleteSOOwnershipTransfer(ctx, host, keys[1], ""); !done || err != nil {
		t.Fatalf("successor did not complete the transfer: %v %v", done, err)
	}
	if done, err := CompleteSOOwnershipTransfer(ctx, host, keys[1], ""); done || err != nil {
		t.Fatalf("completed transfer ran again: %v %v", done, err)
	}

	// The successor owns a Space whose proofs all verify without the departed owner.
	current := *state
	participants := current.GetConfig().GetParticipants()
	if len(participants) != 2 ||
		participants[0].GetPeerId() != peers[1].GetPeerID().String() || !IsOwner(participants[0].GetRole()) ||
		participants[1].GetPeerId() != peers[2].GetPeerID().String() || participants[1].GetRole() != SOParticipantRole_SOParticipantRole_WRITER {
		t.Fatalf("transfer left participants %v", participants)
	}
	if len(current.GetRootGrants()) != 2 {
		t.Fatalf("transfer left %d grants, want 2", len(current.GetRootGrants()))
	}

	// Every remaining proof verifies against the successor's configuration.
	for _, grant := range current.GetRootGrants() {
		if err := grant.ValidateSignature(mockSharedObjectID, participants); err != nil {
			t.Fatalf("grant for %s does not verify: %v", grant.GetPeerId(), err)
		}
	}
	if valid, err := current.GetRoot().ValidateSignatures(mockSharedObjectID, participants); valid == 0 || err != nil {
		t.Fatalf("root does not verify under the successor: %d %v", valid, err)
	}

	// Peers verify the promotion and departure as a relayed suffix.
	history, err := host.ReadConfigHistory(ctx, checkpoint.GetConfigChainHash(), current.GetConfig().GetConfigChainHash())
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyConfigChainSuffix(mockSharedObjectID, checkpoint, current.GetConfig(), history); err != nil {
		t.Fatalf("peers cannot verify the transfer: %v", err)
	}

	// Hosts route to the successor, which can promote its other identity.
	successor, err := ReadSOOwnershipSuccessor(ctx, host)
	if err != nil || successor != peers[1].GetPeerID().String() {
		t.Fatalf("read successor %q: %v", successor, err)
	}
	if err := PromoteSOOwner(ctx, host, keys[1], peers[2].GetPeerID().String()); err != nil {
		t.Fatal(err)
	}
	if !IsOwner((*state).GetConfig().GetParticipants()[1].GetRole()) {
		t.Fatal("promotion did not raise the participant to owner")
	}
	if done, err := CompleteSOOwnershipTransfer(ctx, host, keys[2], ""); done || err != nil {
		t.Fatalf("promotion without departure completed a transfer: %v %v", done, err)
	}
	if successor, err := ReadSOOwnershipSuccessor(ctx, host); err != nil || successor != peers[1].GetPeerID().String() {
		t.Fatalf("promotion without departure moved the successor to %q: %v", successor, err)
	}

	// A remaining participant submits under the new configuration.
	if err := host.QueueOperation(ctx, peers[2].GetPeerID(), func(link *SOOperationLink) (*SOOperation, error) {
		return BuildSOOperation(mockSharedObjectID, keys[2], []byte("after transfer"), link, NewSOOperationLocalID())
	}); err != nil {
		t.Fatal(err)
	}
}

// TestSelectSOSuccessor prefers the highest remaining role, then configuration order.
func TestSelectSOSuccessor(t *testing.T) {
	config := &SharedObjectConfig{Participants: []*SOParticipantConfig{
		{PeerId: "owner", Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: "writer", Role: SOParticipantRole_SOParticipantRole_WRITER},
		{PeerId: "first-validator", Role: SOParticipantRole_SOParticipantRole_VALIDATOR},
		{PeerId: "second-validator", Role: SOParticipantRole_SOParticipantRole_VALIDATOR},
	}}
	if got := selectSOSuccessor(config, []string{"owner"}); got != "first-validator" {
		t.Fatalf("selected %q", got)
	}
	if got := selectSOSuccessor(config, []string{"owner", "first-validator", "second-validator", "writer"}); got != "" {
		t.Fatalf("selected %q with nobody remaining", got)
	}
}
