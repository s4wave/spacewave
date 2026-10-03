package resource_space

import (
	"slices"
	"testing"

	"github.com/s4wave/spacewave/core/resource/space/sharingstate"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// TestSharingGroupControl checks how the sharing state describes group
// control and a change the group has not decided.
func TestSharingGroupControl(t *testing.T) {
	// Three voters share a Space; the viewer and one other agree to remove the third.
	participants := []*sobject.SOParticipantConfig{
		{PeerId: "viewer", VotingWeight: 1},
		{PeerId: "member", VotingWeight: 1},
		{PeerId: "leaving", VotingWeight: 1},
	}
	removal := &sobject.SOConfigChange{
		ChangeType: sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT,
		Config: &sobject.SharedObjectConfig{
			Control:      sobject.SOControl_SO_CONTROL_GROUP,
			Participants: participants[:2],
		},
	}
	state := &sharingstate.SharingState{
		Participants: participants,
		ViewerPeerID: "viewer",
		Control:      sobject.SOControl_SO_CONTROL_GROUP,
		ViewerWeight: 1,
		TotalWeight:  3,
		Agreements: []*sobject.ControlAgreement{{
			Hash:   []byte("removal"),
			Change: removal,
			Voters: []string{"member", "viewer"},
			Weight: 2,
		}},
	}

	// A voter may ask for changes, and all three votes decide one.
	got := sharingStateToProto(state, nil, true)
	if got.GetControl() != s4wave_space.SpaceControl_SpaceControl_GROUP || !got.GetCanVote() || !got.GetCanSetControl() {
		t.Fatalf("control %v, can vote %v, can set %v", got.GetControl(), got.GetCanVote(), got.GetCanSetControl())
	}
	if got.GetQuorumWeight() != 3 {
		t.Fatalf("quorum weight %d, want 3", got.GetQuorumWeight())
	}

	// The waiting change reads as removing the third member, agreed by the viewer.
	changes := got.GetGroupChanges()
	if len(changes) != 1 {
		t.Fatalf("got %d changes", len(changes))
	}
	c := changes[0]
	if !slices.Equal(c.GetRemovedPeerIds(), []string{"leaving"}) || len(c.GetAddedPeerIds()) != 0 {
		t.Fatalf("removed %v, added %v", c.GetRemovedPeerIds(), c.GetAddedPeerIds())
	}
	if c.GetWeight() != 2 || !c.GetViewerAgreed() || c.GetSequencer() != s4wave_space.SpaceSequencer_SpaceSequencer_MERGE {
		t.Fatalf("weight %d, viewer agreed %v, sequencer %v", c.GetWeight(), c.GetViewerAgreed(), c.GetSequencer())
	}

	// A member without a vote watches but cannot change control.
	state.ViewerWeight = 0
	state.CanManage = true
	if got := sharingStateToProto(state, nil, true); got.GetCanSetControl() {
		t.Fatal("an owner without a vote may change group control")
	}
}
