package dexaccess

import (
	"slices"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// TestHolders proves the holders are the readable participants other than
// the local identities, and unknown without a state.
func TestHolders(t *testing.T) {
	// Name a reader, a writer, an owner, a participant that cannot read, and
	// this device.
	ids := make([]peer.ID, 5)
	for i := range ids {
		p, err := peer.NewPeer(nil)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = p.GetPeerID()
	}
	reader, writer, owner, unknown, local := ids[0], ids[1], ids[2], ids[3], ids[4]
	participant := func(id peer.ID, role sobject.SOParticipantRole) *sobject.SOParticipantConfig {
		return &sobject.SOParticipantConfig{PeerId: id.String(), Role: role}
	}

	// Build a config holding each of them and one peer ID that does not parse.
	state := &sobject.SOState{Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
		participant(reader, sobject.SOParticipantRole_SOParticipantRole_READER),
		participant(writer, sobject.SOParticipantRole_SOParticipantRole_WRITER),
		participant(owner, sobject.SOParticipantRole_SOParticipantRole_OWNER),
		participant(unknown, sobject.SOParticipantRole_SOParticipantRole_UNKNOWN),
		participant(local, sobject.SOParticipantRole_SOParticipantRole_OWNER),
		{PeerId: "not-a-peer", Role: sobject.SOParticipantRole_SOParticipantRole_READER},
	}}}

	// Only the readers other than this device hold the blocks.
	if got, want := Holders(state, local), []peer.ID{reader, writer, owner}; !slices.Equal(got, want) {
		t.Fatalf("holders = %v; want %v", got, want)
	}

	// A config with no other reader has no holders, which is not unknown.
	solo := &sobject.SOState{Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
		participant(local, sobject.SOParticipantRole_SOParticipantRole_OWNER),
	}}}
	if got := Holders(solo, local); got == nil || len(got) != 0 {
		t.Fatalf("solo holders = %#v; want empty", got)
	}

	// Without a state the holders are unknown.
	if got := Holders(nil, local); got != nil {
		t.Fatalf("nil state holders = %v; want nil", got)
	}
}
