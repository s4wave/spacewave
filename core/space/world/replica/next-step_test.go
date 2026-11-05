package space_world_replica

import (
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// TestNextStep walks a move of ownership from Device a to Device b, each row
// one observation on one Device, and checks the step of every row of the
// plan's table.
func TestNextStep(t *testing.T) {
	// Name two Devices and the agreed spec of a move from a to b.
	keyA, a := testPeer(t)
	_, b := testPeer(t)
	_, aSession := testPeer(t)
	spec := &ReplicaSpec{SharedObjectId: "so", DesiredOwnerPeerId: b}
	back := &ReplicaSpec{SharedObjectId: "so", DesiredOwnerPeerId: a}

	// Derive the states of ownership the move passes through, and the recheck
	// outcomes.
	aOwns := settled(t, owner(a))
	aOwnsBJoined := settled(t, owner(a), member(b))
	handingOver := handover(t, keyA, owner(a), owner(b))
	bOwns := settled(t, owner(b), member(a))
	complete := &sobject_world_engine.BackfillState{Seqno: 4}
	holes := &sobject_world_engine.BackfillState{Seqno: 4, Holes: 3}

	// Each row is one Device's observation and its step.
	cases := []struct {
		name string
		spec *ReplicaSpec
		obs  Observation
		want ReplicaStep
	}{
		{"no peer spec", nil, Observation{LocalPeerID: a, Ownership: aOwns}, ReplicaStep_REPLICA_STEP_WAIT_PEER},
		{"specs disagree", nil, Observation{LocalPeerID: a, Ownership: aOwns, PeerSpec: &ReplicaSpec{SharedObjectId: "so", DesiredOwnerPeerId: a}}, ReplicaStep_REPLICA_STEP_CONFLICT},
		{"desired owner waits for an invite", nil, Observation{LocalPeerID: b}, ReplicaStep_REPLICA_STEP_WAIT_INVITE},
		{"desired owner joins", nil, Observation{LocalPeerID: b, InviteReceived: true}, ReplicaStep_REPLICA_STEP_JOIN},
		{"owner invites", nil, Observation{LocalPeerID: a, Ownership: aOwns}, ReplicaStep_REPLICA_STEP_INVITE},
		{"desired owner backfills", nil, Observation{LocalPeerID: b, Ownership: aOwnsBJoined}, ReplicaStep_REPLICA_STEP_BACKFILL},
		{"desired owner backfills holes", nil, Observation{LocalPeerID: b, Ownership: aOwnsBJoined, Backfill: holes}, ReplicaStep_REPLICA_STEP_BACKFILL},
		{"desired owner serves its proof", nil, Observation{LocalPeerID: b, Ownership: aOwnsBJoined, Backfill: complete}, ReplicaStep_REPLICA_STEP_SERVE_PROOF},
		{"owner waits for the proof", nil, Observation{LocalPeerID: a, Ownership: aOwnsBJoined, DesiredOwnerParticipates: true}, ReplicaStep_REPLICA_STEP_WAIT_PROOF},
		{"owner transfers", nil, Observation{LocalPeerID: a, Ownership: aOwnsBJoined, DesiredOwnerParticipates: true, ProofAccepted: true}, ReplicaStep_REPLICA_STEP_TRANSFER},
		{"successor completes", nil, Observation{LocalPeerID: b, Ownership: handingOver, Backfill: complete}, ReplicaStep_REPLICA_STEP_COMPLETE_TRANSFER},
		{"departing owner follows", nil, Observation{LocalPeerID: a, Ownership: handingOver, DesiredOwnerParticipates: true, ProofAccepted: true}, ReplicaStep_REPLICA_STEP_FOLLOW},
		{"new owner matches", nil, Observation{LocalPeerID: b, Ownership: bOwns}, ReplicaStep_REPLICA_STEP_NONE},
		{"old owner matches", nil, Observation{LocalPeerID: a, Ownership: bOwns, DesiredOwnerParticipates: true}, ReplicaStep_REPLICA_STEP_NONE},
		{"co-owner beside the desired owner", nil, Observation{LocalPeerID: a, Ownership: settled(t, owner(a), owner(b)), DesiredOwnerParticipates: true}, ReplicaStep_REPLICA_STEP_NONE},
		{"no copy and not desired", nil, Observation{LocalPeerID: a}, ReplicaStep_REPLICA_STEP_NONE},
		{"creator owns as its storage peer", nil, Observation{LocalPeerID: aSession, StoragePeerID: a, Ownership: aOwns}, ReplicaStep_REPLICA_STEP_INVITE},
		{"departed owner waits for an invite", back, Observation{LocalPeerID: a, Ownership: handingOver}, ReplicaStep_REPLICA_STEP_WAIT_INVITE},
		{"departed owner rejoins", back, Observation{LocalPeerID: a, Ownership: handingOver, InviteReceived: true}, ReplicaStep_REPLICA_STEP_JOIN},
	}

	// Check each row with the agreed spec unless the row sets its own.
	for _, tc := range cases {
		obs := tc.obs
		obs.Spec = spec
		if tc.spec != nil {
			obs.Spec = tc.spec
		}
		if obs.PeerSpec == nil && tc.want != ReplicaStep_REPLICA_STEP_WAIT_PEER {
			obs.PeerSpec = obs.Spec.CloneVT()
		}
		if got := NextStep(&obs); got != tc.want {
			t.Errorf("%s: step %v, want %v", tc.name, got, tc.want)
		}
	}
}

// testPeer returns a new Device key and its peer ID.
func testPeer(t *testing.T) (crypto.PrivKey, string) {
	// Generate the key.
	t.Helper()
	key, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Derive its peer ID.
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return key, id.String()
}

// owner returns an owner participant.
func owner(id string) *sobject.SOParticipantConfig {
	return &sobject.SOParticipantConfig{PeerId: id, Role: sobject.SOParticipantRole_SOParticipantRole_OWNER}
}

// member returns a writer participant.
func member(id string) *sobject.SOParticipantConfig {
	return &sobject.SOParticipantConfig{PeerId: id, Role: sobject.SOParticipantRole_SOParticipantRole_WRITER}
}

// settled derives ownership of a config with no transfer at its head.
func settled(t *testing.T, participants ...*sobject.SOParticipantConfig) *sobject.SOOwnership {
	// Derive with no head change.
	t.Helper()
	own, err := sobject.DeriveSOOwnership(&sobject.SharedObjectConfig{Participants: participants}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return own
}

// handover derives ownership of a config whose head transfers ownership with
// the departure of departing's key.
func handover(t *testing.T, departing crypto.PrivKey, participants ...*sobject.SOParticipantConfig) *sobject.SOOwnership {
	// Sign the departure at a config head.
	t.Helper()
	request, err := sobject.BuildSOLeaveRequest("so", make([]byte, 32), departing)
	if err != nil {
		t.Fatal(err)
	}

	// Derive ownership with the transfer at the head.
	head := &sobject.SOConfigChange{
		ChangeType:   sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_TRANSFER_OWNERSHIP,
		LeaveRequest: request,
	}
	own, err := sobject.DeriveSOOwnership(&sobject.SharedObjectConfig{Participants: participants}, head)
	if err != nil {
		t.Fatal(err)
	}
	return own
}
