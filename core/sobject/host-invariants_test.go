package sobject

import (
	"bytes"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/hash"
)

// TestImportHistoricalRoot retains the proof accepted before an owner departed.
func TestImportHistoricalRoot(t *testing.T) {
	// Hold a root signed by an owner who will depart.
	peers := createMockPeers(t, 3)
	previous := createMockSOState(peers, []SOParticipantRole{
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_READER,
	})
	previous.Config.ConfigChainHash = bytes.Repeat([]byte{1}, 32)
	previous.Root = createMockSORoot(t, 1, peers[0])
	owner, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	// The departing owner removes itself.
	candidate := previous.CloneVT()
	candidate.Config.Participants = candidate.Config.Participants[1:]
	entry, err := BuildSOConfigChange(mockSharedObjectID, previous.Config, candidate.Config,
		SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Config, err = VerifyConfigChange(mockSharedObjectID, previous.Config, entry)
	if err != nil {
		t.Fatal(err)
	}

	// A reader imports the snapshot and keeps the accepted root.
	host, current := newTestSOHost(t.Context(), previous)
	if err := host.ImportPeerSnapshot(t.Context(), candidate, []*SOConfigChange{entry}, peers[2].GetPeerID(), nil); err != nil {
		t.Fatal(err)
	}
	if !(*current).Root.EqualVT(previous.Root) || !EqualSOConfigs((*current).Config, candidate.Config) {
		t.Fatal("import did not preserve the accepted root and advance configuration")
	}
}

// TestImportProofAfterPartialSync accepts a proof whose prefix this replica
// already applied through ordinary sync.
func TestImportProofAfterPartialSync(t *testing.T) {
	// Use two owners and a reader.
	peers := createMockPeers(t, 3)
	previous := createMockSOState(peers, []SOParticipantRole{
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_READER,
	})
	previous.Config.ConfigChainHash = bytes.Repeat([]byte{1}, 32)
	owner, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	// The owner removes the reader and then peer 1; the replica has synced only the first.
	var entries []*SOConfigChange
	config := previous.Config
	for range 2 {
		next := config.CloneVT()
		next.Participants = next.Participants[:len(next.Participants)-1]
		entry, err := BuildSOConfigChange(mockSharedObjectID, config, next,
			SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, owner, nil)
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
	synced.Config, err = VerifyConfigChange(mockSharedObjectID, previous.Config, entries[0])
	if err != nil {
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

// TestImportRootNonceRollback rejects omission or rollback in a newer signed root.
func TestImportRootNonceRollback(t *testing.T) {
	// Hold a root with a head at nonce 7.
	peers := createMockPeers(t, 1)
	previous := createMockSOState(peers, nil)
	previous.Config.ConfigChainHash = bytes.Repeat([]byte{1}, 32)
	previous.Root = createMockSORoot(t, 1, peers[0])
	previous.Root.AccountNonces = []*SOAccountNonce{{PeerId: peers[0].GetPeerID().String(), Nonce: 7, OpHash: mockPrevOpHash}}
	previous.Root.ValidatorSignatures = nil

	// Sign it as the owner.
	owner, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := previous.Root.SignInnerData(owner, mockSharedObjectID, 1, hash.RecommendedHashType); err != nil {
		t.Fatal(err)
	}

	// A newer root that lowers or drops the head is refused.
	for _, omitted := range []bool{false, true} {
		candidate := previous.CloneVT()
		candidate.Root = createMockSORoot(t, 2, peers[0])
		candidate.Root.AccountNonces = []*SOAccountNonce{{PeerId: peers[0].GetPeerID().String(), Nonce: 6, OpHash: mockPrevOpHash}}
		if omitted {
			candidate.Root.AccountNonces = nil
		}
		candidate.Root.ValidatorSignatures = nil
		if err := candidate.Root.SignInnerData(owner, mockSharedObjectID, 2, hash.RecommendedHashType); err != nil {
			t.Fatal(err)
		}
		host, current := newTestSOHost(t.Context(), previous)
		if err := host.ImportPeerSnapshot(t.Context(), candidate, nil, peers[0].GetPeerID(), nil); err == nil {
			t.Fatalf("accepted nonce rollback, omitted=%v", omitted)
		}
		if !(*current).EqualVT(previous) {
			t.Fatal("rejected import changed the held state")
		}
	}
}

// TestPeerSnapshotExchangeConverges advances the older peer and retains local invitations.
func TestPeerSnapshotExchangeConverges(t *testing.T) {
	peers := createMockPeers(t, 2)
	older := createMockSOState(peers, nil)
	older.Config.ConfigChainHash = bytes.Repeat([]byte{1}, 32)
	older.Root = createMockSORoot(t, 1, peers[0])
	older.Invites = []*SOInvite{{InviteId: "older peer invitation"}}
	newer := older.CloneVT()
	newer.Root = createMockSORoot(t, 3, peers[0])
	newer.Invites = []*SOInvite{{InviteId: "newer peer invitation"}}
	left, leftState := newTestSOHost(t.Context(), older)
	right, rightState := newTestSOHost(t.Context(), newer)
	if err := left.ImportPeerSnapshot(t.Context(), newer, nil, peers[0].GetPeerID(), nil); err != nil {
		t.Fatal(err)
	}
	if err := right.ImportPeerSnapshot(t.Context(), older, nil, peers[1].GetPeerID(), nil); err == nil {
		t.Fatal("reverse exchange accepted an older root")
	}
	if !EqualSOConfigs((*leftState).Config, (*rightState).Config) || !(*leftState).Root.EqualVT((*rightState).Root) {
		t.Fatal("peers did not converge to the newer checkpoint")
	}
	if (*leftState).Invites[0].InviteId != older.Invites[0].InviteId || (*rightState).Invites[0].InviteId != newer.Invites[0].InviteId {
		t.Fatal("snapshot exchange replaced local invitation capabilities")
	}
}

// TestImportRequeuesAfterClearedRejection keeps a pending local operation
// whose preceding nonce a since-cleared rejection consumed.
func TestImportRequeuesAfterClearedRejection(t *testing.T) {
	// Hold a single-writer state at an accepted root.
	peers := createMockPeers(t, 1)
	previous := createMockSOState(peers, nil)
	previous.Config.ConfigChainHash = bytes.Repeat([]byte{1}, 32)
	previous.Root = createMockSORoot(t, 1, peers[0])
	priv, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	// Nonce 1 was rejected and cleared; nonce 2 waits for the validator.
	previous.QueuedAccountNonces = []*SOAccountNonce{{PeerId: peers[0].GetPeerID().String(), Nonce: 1, OpHash: mockPrevOpHash}}
	op, err := BuildSOOperation(mockSharedObjectID, priv, []byte("two"), linkAt(previous, priv, 2), NewSOOperationLocalID())
	if err != nil {
		t.Fatal(err)
	}
	if err := previous.QueueOperation(mockSharedObjectID, op); err != nil {
		t.Fatal(err)
	}

	// The validator's snapshot has neither the operation nor the rejection.
	candidate := previous.CloneVT()
	candidate.Ops = nil
	candidate.QueuedAccountNonces = nil
	host, current := newTestSOHost(t.Context(), previous)
	if err := host.ImportPeerSnapshot(t.Context(), candidate, nil, peers[0].GetPeerID(), nil); err != nil {
		t.Fatal(err)
	}
	if len((*current).GetOps()) != 1 || !(*current).GetOps()[0].EqualVT(op) {
		t.Fatal("import dropped the pending local operation")
	}
}
