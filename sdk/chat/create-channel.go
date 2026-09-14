package spacewave_chat

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
	"github.com/sirupsen/logrus"
)

// CreateChatChannelOpId is the operation id for CreateChatChannelOp.
var CreateChatChannelOpId = "spacewave-chat/channel/create"

// NewCreateChatChannelOpBlock constructs a new CreateChatChannelOp block.
func NewCreateChatChannelOpBlock() block.Block {
	return &CreateChatChannelOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *CreateChatChannelOp) GetOperationTypeId() string {
	return CreateChatChannelOpId
}

// Validate performs cursory checks on the op.
func (o *CreateChatChannelOp) Validate() error {
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	for _, state := range o.GetInitialState() {
		if err := state.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// MarshalBlock marshals the block to binary.
func (o *CreateChatChannelOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *CreateChatChannelOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateChatChannelOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	objKey := o.GetObjectKey()
	channel := &ChatChannel{
		Name:                      o.GetName(),
		Topic:                     o.GetTopic(),
		CreatedAt:                 o.GetTimestamp(),
		CreatorPeerId:             sender.String(),
		EncryptionAlgorithm:       o.GetEncryptionAlgorithm(),
		ThreadIndexedMessageCount: new(uint64),
	}

	{
		createdObject, _, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
			bcs.SetBlock(channel, true)
			return nil
		})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			return false, err
		}
	}

	if err := world_types.SetObjectType(ctx, ws, objKey, ChatChannelTypeID); err != nil {
		return false, err
	}

	// Initial events share the operation's author, timestamp, and transaction.
	resource := NewChatResource(ws, nil, objKey, sender.String())
	for _, state := range o.GetInitialState() {
		request := &spacewave_chat_rpc.SendMessageRequest{
			Content: &ChatMessageContent{Content: &ChatMessageContent_StateChange{StateChange: state}},
		}
		if _, err := resource.appendMessage(ctx, ws, request, o.GetTimestamp()); err != nil {
			return false, err
		}
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object.
func (o *CreateChatChannelOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// LookupCreateChatChannelOp looks up a CreateChatChannelOp operation type.
func LookupCreateChatChannelOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == CreateChatChannelOpId {
		return &CreateChatChannelOp{}, nil
	}
	return nil, nil
}

var _ world.Operation = (*CreateChatChannelOp)(nil)
