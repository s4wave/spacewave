package sobject

import "testing"

// TestRotateTransformKeyExhaustion rejects an epoch counter that would wrap
// into an earlier generation.
func TestRotateTransformKeyExhaustion(t *testing.T) {
	peers := createMockPeers(t, 1)
	key := mustPrivKeys(t, peers)[0]
	state, _ := newTestSOState(t, peers)
	if _, _, err := RotateTransformKey(key, mockSharedObjectID, state.GetConfig().GetParticipants(), ^uint64(0), 0); err == nil {
		t.Fatal("rotation wrapped the exhausted epoch counter")
	}
}

// TestRotateTransformKeyGrantsReaders checks that a rotation grants the next
// epoch to every reader and to no one else.
func TestRotateTransformKeyGrantsReaders(t *testing.T) {
	// Rotate the key for an owner and a non-reader.
	peers := createMockPeers(t, 2)
	key := mustPrivKeys(t, peers)[0]
	participants := []*SOParticipantConfig{
		{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER},
		{PeerId: peers[1].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_UNKNOWN},
	}
	conf, epoch, err := RotateTransformKey(key, mockSharedObjectID, participants, 4, 7)
	if err != nil {
		t.Fatal(err)
	}
	if epoch.GetEpoch() != 5 || epoch.GetConfigChainSeqno() != 7 || len(epoch.GetGrants()) != 1 || epoch.FindGrant(participants[0].GetPeerId()) == nil {
		t.Fatalf("rotation built epoch %d at config %d with %d grants; want epoch 5 at config 7 for the owner", epoch.GetEpoch(), epoch.GetConfigChainSeqno(), len(epoch.GetGrants()))
	}
	inner, err := epoch.GetGrants()[0].DecryptInnerData(key, mockSharedObjectID)
	if err != nil || !inner.GetTransformConf().EqualVT(conf) {
		t.Fatalf("grant does not carry the new key: %v", err)
	}
}

// TestNeedsKeyRotation checks that a reader removed after the latest epoch's
// config needs a new key, and that a later epoch or an addition does not.
func TestNeedsKeyRotation(t *testing.T) {
	// Build a chain that adds a reader, then removes it.
	peers := createMockPeers(t, 2)
	owner := &SOParticipantConfig{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER}
	reader := &SOParticipantConfig{PeerId: peers[1].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_READER}
	record := func(seqno uint64, participants ...*SOParticipantConfig) *SOConfigChange {
		return &SOConfigChange{ConfigSeqno: seqno, Config: &SharedObjectConfig{Participants: participants}}
	}
	history := []*SOConfigChange{record(0, owner), record(1, owner, reader), record(2, owner)}

	// The epoch of config 1 is held by the removed reader.
	if !NeedsKeyRotation(history, []*SOKeyEpoch{{Epoch: 1}, {Epoch: 2, ConfigChainSeqno: 1}}) {
		t.Fatal("removed reader keeps the latest key")
	}

	// The epoch of config 2 already replaced it.
	if NeedsKeyRotation(history, []*SOKeyEpoch{{Epoch: 3, ConfigChainSeqno: 2}, {Epoch: 2, ConfigChainSeqno: 1}}) {
		t.Fatal("rotated again after the replacing epoch")
	}

	// Adding a reader needs no rotation.
	if NeedsKeyRotation(history[:2], []*SOKeyEpoch{{Epoch: 1}}) {
		t.Fatal("rotated after an addition")
	}
}
