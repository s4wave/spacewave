package resource_space

import (
	"testing"

	"github.com/s4wave/spacewave/core/resource/space/sharingstate"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// TestSharingSequencer checks how the sharing state names the appointed
// sequencer to the viewer.
func TestSharingSequencer(t *testing.T) {
	// The viewer and one other member device share a Space.
	participants := []*sobject.SOParticipantConfig{{PeerId: "viewer"}, {PeerId: "member"}}
	cloud := []s4wave_space.SpaceSequencer{
		s4wave_space.SpaceSequencer_SpaceSequencer_MERGE,
		s4wave_space.SpaceSequencer_SpaceSequencer_PROVIDER,
	}
	local := []s4wave_space.SpaceSequencer{
		s4wave_space.SpaceSequencer_SpaceSequencer_MERGE,
		s4wave_space.SpaceSequencer_SpaceSequencer_THIS_DEVICE,
	}
	cases := []struct {
		name    string
		peerID  string
		choices []s4wave_space.SpaceSequencer
		want    s4wave_space.SpaceSequencer
	}{
		{"merge", "", cloud, s4wave_space.SpaceSequencer_SpaceSequencer_MERGE},
		{"this device", "viewer", local, s4wave_space.SpaceSequencer_SpaceSequencer_THIS_DEVICE},
		{"other device", "member", local, s4wave_space.SpaceSequencer_SpaceSequencer_OTHER_DEVICE},
		{"provider", "cloud", cloud, s4wave_space.SpaceSequencer_SpaceSequencer_PROVIDER},
		{"departed device", "gone", local, s4wave_space.SpaceSequencer_SpaceSequencer_OTHER_DEVICE},
	}

	// Each appointment reads as the viewer would describe it.
	for _, tc := range cases {
		state := &sharingstate.SharingState{
			Participants:    participants,
			ViewerPeerID:    "viewer",
			SequencerPeerID: tc.peerID,
		}
		if got := sharingSequencer(state, tc.choices); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
