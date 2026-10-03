package sobject

import "testing"

// TestRemovalMissingSigner rejects absent proof keys without panicking or publishing a clone.
func TestRemovalMissingSigner(t *testing.T) {
	// Build a state with a member and an owner.
	peers := createMockPeers(t, 2)
	owner, err := peers[1].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	initial, _ := newTestSOState(t, peers,
		SOParticipantRole_SOParticipantRole_OWNER,
		SOParticipantRole_SOParticipantRole_OWNER,
	)
	for _, kind := range []string{"grant", "checkpoint", "nil-grant"} {
		t.Run(kind, func(t *testing.T) {
			// Break one part of the removal proof.
			state := initial.CloneVT()
			switch kind {
			case "grant":
				state.KeyEpochs[0].Grants[1].Signature.PubKey = nil
			case "checkpoint":
				state.Checkpoint.Signatures[0].PubKey = nil
			case "nil-grant":
				state.KeyEpochs[0].Grants = append(state.KeyEpochs[0].Grants, nil)
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
