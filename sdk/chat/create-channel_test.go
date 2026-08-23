package spacewave_chat

import (
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

func TestCreateChatChannelOpRejectsInvalidMembers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		members []string
	}{
		{"empty member id", []string{"peer-a", ""}},
		{"duplicate member id", []string{"peer-a", "peer-b", "peer-a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			op := &CreateChatChannelOp{
				ObjectKey:     "chat/channel/invalid",
				Name:          "Broken",
				Timestamp:     timestamppb.Now(),
				MemberPeerIds: tc.members,
			}
			if err := op.Validate(); err == nil {
				t.Fatalf("Validate with %v = nil, want an error", tc.members)
			}
		})
	}
}
