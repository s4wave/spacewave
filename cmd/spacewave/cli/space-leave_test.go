//go:build !js

package spacewave_cli

import (
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// TestSuccessorPeerID checks that a member name selects that member's
// strongest peer, a peer ID selects itself, and the viewer is refused.
func TestSuccessorPeerID(t *testing.T) {
	// One owner leaving, and a member with a reader and two writer peers.
	state := &s4wave_space.SpaceSharingState{
		Participants: []*sobject.SOParticipantConfig{
			{PeerId: "self", Role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
			{PeerId: "m-reader", Role: sobject.SOParticipantRole_SOParticipantRole_READER},
			{PeerId: "m-writer-1", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
			{PeerId: "m-writer-2", Role: sobject.SOParticipantRole_SOParticipantRole_WRITER},
		},
		ParticipantInfo: []*s4wave_space.SpaceParticipantInfo{
			{AccountId: "acct-self", PeerIds: []string{"self"}, IsSelf: true},
			{AccountId: "acct-m", EntityId: "m", PeerIds: []string{"m-reader", "m-writer-1", "m-writer-2"}},
		},
	}

	// Names pick the strongest, earliest peer. A peer ID is taken as given.
	for name, want := range map[string]string{
		"acct-m":     "m-writer-1",
		"m":          "m-writer-1",
		"m-reader":   "m-reader",
		"m-writer-2": "m-writer-2",
	} {
		got, err := successorPeerID(state, name)
		if err != nil || got != want {
			t.Fatalf("%s: got %q, %v; want %q", name, got, err, want)
		}
	}

	// The viewer and strangers are refused.
	for _, name := range []string{"acct-self", "self", "nobody"} {
		if got, err := successorPeerID(state, name); err == nil {
			t.Fatalf("%s: got %q, want an error", name, got)
		}
	}
}
