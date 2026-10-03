package sobject

import (
	"bytes"
	"testing"

	"github.com/pkg/errors"
)

// TestImportKeepsCheckpointAfterSignerLeaves checks that a reader keeps the
// held checkpoint, with proofs the remaining owner signed, when the owner who
// signed it removes itself.
func TestImportKeepsCheckpointAfterSignerLeaves(t *testing.T) {
	// Two owners and a reader hold the genesis checkpoint signed by the first owner.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	previous, _ := newTestSOState(t, peers,
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_READER,
	)

	// The first owner removes itself.
	candidate := previous.CloneVT()
	candidate.Config.Participants = candidate.Config.Participants[1:]
	entry, err := BuildSOConfigChange(mockSharedObjectID, previous.Config, candidate.Config,
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Config, err = VerifyConfigChange(mockSharedObjectID, previous.Config, entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := pruneRemovedParticipants(mockSharedObjectID, candidate, map[string]struct{}{peers[0].GetPeerID().String(): {}}, keys[1]); err != nil {
		t.Fatal(err)
	}

	// The reader imports the snapshot and keeps the checkpoint.
	host, current := newTestSOHost(t.Context(), previous)
	if err := host.ImportPeerSnapshot(t.Context(), candidate, []*SOConfigChange{entry}, peers[2].GetPeerID(), nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal((*current).GetCheckpoint().Hash(), previous.GetCheckpoint().Hash()) || !EqualSOConfigs((*current).Config, candidate.Config) {
		t.Fatal("import did not keep the checkpoint and advance the configuration")
	}
	if err := (*current).ValidateAuthority(mockSharedObjectID); err != nil {
		t.Fatalf("import kept proofs of the departed owner: %v", err)
	}
}

// TestImportProofAfterPartialSync accepts a proof whose prefix this replica
// already applied through ordinary sync.
func TestImportProofAfterPartialSync(t *testing.T) {
	// Use two owners and a reader.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	previous, _ := newTestSOState(t, peers,
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_READER,
	)

	// The owner removes the reader and then peer 1; the replica has synced only the first.
	var entries []*SOConfigChange
	config := previous.Config
	for range 2 {
		next := config.CloneVT()
		next.Participants = next.Participants[:len(next.Participants)-1]
		entry, err := BuildSOConfigChange(mockSharedObjectID, config, next,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, keys[0], nil)
		if err != nil {
			t.Fatal(err)
		}
		config, err = VerifyConfigChange(mockSharedObjectID, config, entry)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}

	// The replica holds the first removal and receives both.
	synced := previous.CloneVT()
	var err error
	synced.Config, err = VerifyConfigChange(mockSharedObjectID, previous.Config, entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := pruneRemovedParticipants(mockSharedObjectID, synced, map[string]struct{}{peers[2].GetPeerID().String(): {}}, keys[0]); err != nil {
		t.Fatal(err)
	}
	candidate := synced.CloneVT()
	candidate.Config = config

	// Import applies the unsynced removal and reports the revocation.
	host, current := newTestSOHost(t.Context(), synced)
	err = host.ImportPeerSnapshot(t.Context(), candidate, entries, peers[1].GetPeerID(), nil)
	if !errors.Is(err, ErrParticipantRevoked) {
		t.Fatalf("expected committed revocation, got %v", err)
	}
	if !EqualSOConfigs((*current).Config, candidate.Config) {
		t.Fatal("import did not apply the unsynced remainder of the proof")
	}
}

// TestPeerSnapshotExchangeConverges checks that two peers who exchange
// snapshots hold the newer checkpoint and every operation, and keep their own
// invitations.
func TestPeerSnapshotExchangeConverges(t *testing.T) {
	// The left peer writes; the right peer advances the checkpoint.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	genesis, _ := newTestSOState(t, peers)
	older, newer := genesis.CloneVT(), genesis.CloneVT()
	writeTestOp(t, older, keys[1], "pending")
	advanceTestCheckpoint(t, newer, keys[0])
	older.Invites = []*SOInvite{{InviteId: "older peer invitation"}}
	newer.Invites = []*SOInvite{{InviteId: "newer peer invitation"}}

	// Each imports the other's snapshot.
	left, leftState := newTestSOHost(t.Context(), older)
	right, rightState := newTestSOHost(t.Context(), newer)
	if err := left.ImportPeerSnapshot(t.Context(), newer, nil, peers[1].GetPeerID(), nil); err != nil {
		t.Fatal(err)
	}
	if err := right.ImportPeerSnapshot(t.Context(), older, nil, peers[0].GetPeerID(), nil); err != nil {
		t.Fatal(err)
	}

	// Both hold the newer checkpoint, the pending operation and their own invitations.
	for _, state := range []*SOState{*leftState, *rightState} {
		if !state.GetCheckpoint().EqualVT(newer.GetCheckpoint()) {
			t.Fatal("peers did not converge to the newer checkpoint")
		}
		if len(state.GetOps()) != 1 || !state.GetOps()[0].EqualVT(older.GetOps()[0]) {
			t.Fatal("peers did not converge to the pending operation")
		}
	}
	if (*leftState).Invites[0].InviteId != older.Invites[0].InviteId || (*rightState).Invites[0].InviteId != newer.Invites[0].InviteId {
		t.Fatal("snapshot exchange replaced local invitation capabilities")
	}
}
