package sobject

import (
	"bytes"
	"testing"
)

// TestRemovalMissingSigner rejects absent proof keys without panicking or publishing a clone.
func TestRemovalMissingSigner(t *testing.T) {
	peers := createMockPeers(t, 2)
	creator, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	owner, err := peers[1].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	initial := createMockSOState(peers, []SOParticipantRole{
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_OWNER,
	})
	initial.Config.ConfigChainHash = bytes.Repeat([]byte{1}, 32)
	initial.Root = createMockSORoot(t, 1, peers[0])
	_, initial.RootGrants, _, err = RotateTransformKey(creator, mockSharedObjectID, initial.Config.Participants, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"grant", "root", "nil-grant"} {
		t.Run(kind, func(t *testing.T) {
			state := initial.CloneVT()
			switch kind {
			case "grant":
				state.RootGrants[1].Signature.PubKey = nil
			case "root":
				state.Root.ValidatorSignatures[0].PubKey = nil
			case "nil-grant":
				state.RootGrants = append(state.RootGrants, nil)
			}
			host, held := newTestSOHost(t.Context(), state)
			if _, err := RemoveSOParticipants(t.Context(), host, []string{peers[0].GetPeerID().String()}, owner, nil); err == nil {
				t.Fatal("accepted a proof without a signer public key")
			}
			if !(*held).EqualVT(state) {
				t.Fatal("rejected removal changed the held checkpoint")
			}
		})
	}
}
