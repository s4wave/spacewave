package spacewave_chat

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// InitChatDemoOpId is the operation type ID.
var InitChatDemoOpId = "chat/init-chat-demo"

// GeneralChannelKey is the default channel object key.
const GeneralChannelKey = "chat/channel/general"

// NewInitChatDemoOpBlock creates an empty block for deserialization.
func NewInitChatDemoOpBlock() block.Block {
	return &InitChatDemoOp{}
}

// GetOperationTypeId returns the operation type ID.
func (o *InitChatDemoOp) GetOperationTypeId() string {
	return InitChatDemoOpId
}

// Validate validates the operation.
func (o *InitChatDemoOp) Validate() error {
	return nil
}

// MarshalBlock marshals the operation to bytes.
func (o *InitChatDemoOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the operation from bytes.
func (o *InitChatDemoOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// ApplyWorldOp applies the init chat demo operation.
func (o *InitChatDemoOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (bool, error) {
	objKey := o.GetChannelObjectKey()
	if objKey == "" {
		objKey = GeneralChannelKey
	}

	channel := &ChatChannel{
		Name:      "General",
		CreatedAt: o.GetTimestamp(),
	}
	if _, _, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(channel, true)
		return nil
	}); err != nil {
		return true, err
	}
	if err := world_types.SetObjectType(ctx, ws, objKey, ChatChannelTypeID); err != nil {
		return true, err
	}

	return false, nil
}

// ApplyWorldObjectOp is not supported for this operation.
func (o *InitChatDemoOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (bool, error) {
	return false, world.ErrUnhandledOp
}

// LookupInitChatDemoOp looks up the init chat demo operation.
func LookupInitChatDemoOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID == InitChatDemoOpId {
		return &InitChatDemoOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = (*InitChatDemoOp)(nil)
