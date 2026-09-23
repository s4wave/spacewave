package sobject

import (
	"bytes"
	"testing"
)

// TestConfigChangeMissingSigner rejects omitted keys before deriving a signer identity.
func TestConfigChangeMissingSigner(t *testing.T) {
	peers := createMockPeers(t, 2)
	owner, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	enrolling, err := peers[1].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	current := &SharedObjectConfig{
		Participants:    []*SOParticipantConfig{{PeerId: peers[0].GetPeerID().String(), Role: SOParticipantRole_SOParticipantRole_OWNER, EntityId: "entity"}},
		ConfigChainHash: bytes.Repeat([]byte{1}, 32), ConfigChainSeqno: 1,
	}
	before := current.CloneVT()
	invite, err := BuildSOConfigChange(current, current, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	selfEnroll, err := BuildSelfEnrollPeerConfigChange(current, enrolling, peers[1].GetPeerID().String(), "entity", SOParticipantRole_SOParticipantRole_READER)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []*SOConfigChange{invite, selfEnroll} {
		entry.Signature.PubKey = nil
		if _, err := VerifyConfigChange(current, entry); err == nil {
			t.Fatal("configuration change accepted a missing signer key")
		}
		if !current.EqualVT(before) {
			t.Fatal("rejected configuration change mutated its checkpoint")
		}
	}
}
