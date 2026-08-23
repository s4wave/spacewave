package spacewave_chat_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	spacewave_chat_channel "github.com/s4wave/spacewave/sdk/chat/channel"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// ChatChannelType is the ObjectType for chat channel objects.
var ChatChannelType = objecttype.NewObjectType(spacewave_chat.ChatChannelTypeID, ChatChannelFactory)

// ChatMessageType is the ObjectType for chat message objects.
var ChatMessageType = objecttype.NewObjectType(spacewave_chat.ChatMessageTypeID, chatReadOnlyFactory)

// ChatChannelFactory creates a ChatResource from a channel world object.
func ChatChannelFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}
	peerID := objecttype.SessionPeerIDFromContext(ctx)

	resource := spacewave_chat_channel.NewChatResource(ws, engine, objectKey, peerID.String())
	return resource.GetMux(), resource.Close, nil
}

// chatReadOnlyFactory is a minimal factory for read-only chat object types.
// Viewers access block state through the objectState prop directly.
func chatReadOnlyFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}
	return nil, func() {}, nil
}
