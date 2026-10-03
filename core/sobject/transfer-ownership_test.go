package sobject

import (
	"testing"

	"github.com/sirupsen/logrus"
)

// TestTransferSOOwnershipOwnerDeparture proves that a Space continues after its
// owner leaves: the default successor commits the carried departure, every
// remaining grant and the checkpoint verify under the new configuration, and a
// remaining participant can still submit operations.
func TestTransferSOOwnershipOwnerDeparture(t *testing.T) {
	// Hold an owner and two writers, with the key and checkpoint the owner signed.
	ctx := t.Context()
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	initial, genesis := newTestSOState(t, peers)
	base := initial.GetConfig()
	host, state := newLeaveTestHost(t, base, genesis)
	*state = initial

	// The owner alone cannot leave anyone dependent on its proofs.
	request, err := BuildSOLeaveRequest(mockSharedObjectID, base.GetConfigChainHash(), keys[0])
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
	if len(current.CurrentKeyEpoch().GetGrants()) != 2 {
		t.Fatalf("transfer left %d grants, want 2", len(current.CurrentKeyEpoch().GetGrants()))
	}

	// Every remaining proof verifies against the successor's configuration.
	if err := current.ValidateAuthority(mockSharedObjectID); err != nil {
		t.Fatalf("proofs do not verify under the successor: %v", err)
	}

	// Peers verify the promotion and departure as a relayed suffix.
	history, err := host.ReadConfigHistory(ctx, base.GetConfigChainHash(), current.GetConfig().GetConfigChainHash())
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyConfigChainSuffix(mockSharedObjectID, base, current.GetConfig(), history); err != nil {
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
	le := logrus.NewEntry(logrus.New())
	if _, _, err := host.AddLocalOperation(ctx, le, testStepFactorySet(), keys[2], []byte("after transfer")); err != nil {
		t.Fatal(err)
	}
}

// TestSelectSOSuccessor prefers the highest remaining role, then configuration order.
func TestSelectSOSuccessor(t *testing.T) {
	config := &SharedObjectConfig{Participants: []*SOParticipantConfig{
		{PeerId: "owner", Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: "reader", Role: SOParticipantRole_SOParticipantRole_READER},
		{PeerId: "first-writer", Role: SOParticipantRole_SOParticipantRole_WRITER},
		{PeerId: "second-writer", Role: SOParticipantRole_SOParticipantRole_WRITER},
	}}
	if got := selectSOSuccessor(config, []string{"owner"}); got != "first-writer" {
		t.Fatalf("selected %q", got)
	}
	if got := selectSOSuccessor(config, []string{"owner", "first-writer", "second-writer", "reader"}); got != "" {
		t.Fatalf("selected %q with nobody remaining", got)
	}
}
