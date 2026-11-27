package spacewave_chat

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// GeneralChannelKey is the default channel object key.
const GeneralChannelKey = "chat/channel/general"

// ApplyWorldOp applies the init chat demo operation.
func (o *InitChatDemoOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	// Resolve the demo channel key, using the default key when omitted.
	objKey := o.GetChannelObjectKey()
	if objKey == "" {
		objKey = GeneralChannelKey
	}

	// Create the demo channel with an initialized thread index.
	channel := &ChatChannel{
		Name:                      "General",
		CreatedAt:                 o.GetTimestamp(),
		ThreadIndexedMessageCount: new(uint64),
	}
	{
		createdObject, _, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
			bcs.SetBlock(channel, true)
			return nil
		})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			return true, err
		}
	}
	if err := world_types.SetObjectType(ctx, ws, objKey, ChatChannelTypeID); err != nil {
		return true, err
	}

	return false, nil
}

var _ world.Operation = (*InitChatDemoOp)(nil)
