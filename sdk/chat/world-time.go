package spacewave_chat

import (
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
)

// WorldTime returns when the channel was made, which object browsers show as
// its age.
func (c *ChatChannel) WorldTime() *timestamppb.Timestamp {
	return c.GetCreatedAt()
}

// _ is a type assertion
var _ world.DatedBlock = (*ChatChannel)(nil)
