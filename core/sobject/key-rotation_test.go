package sobject

import "testing"

// TestRotateTransformKeyExhaustion rejects an epoch counter that would wrap
// into an earlier generation.
func TestRotateTransformKeyExhaustion(t *testing.T) {
	peers := createMockPeers(t, 1)
	key := mustPrivKeys(t, peers)[0]
	state, _ := newTestSOState(t, peers)
	if _, _, err := RotateTransformKey(key, mockSharedObjectID, state.GetConfig().GetParticipants(), ^uint64(0)); err == nil {
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
	conf, epoch, err := RotateTransformKey(key, mockSharedObjectID, participants, 4)
	if err != nil {
		t.Fatal(err)
	}
	if epoch.GetEpoch() != 5 || len(epoch.GetGrants()) != 1 || epoch.FindGrant(participants[0].GetPeerId()) == nil {
		t.Fatalf("rotation built epoch %d with %d grants; want epoch 5 for the owner", epoch.GetEpoch(), len(epoch.GetGrants()))
	}
	inner, err := epoch.GetGrants()[0].DecryptInnerData(key, mockSharedObjectID)
	if err != nil || !inner.GetTransformConf().EqualVT(conf) {
		t.Fatalf("grant does not carry the new key: %v", err)
	}
}
