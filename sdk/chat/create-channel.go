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

// AuthenticatedOperation selects the verified transaction signer on replay.
func (o *CreateChatChannelOp) AuthenticatedOperation() {}

// Validate performs cursory checks on the op.
func (o *CreateChatChannelOp) Validate() error {
	// Require a key, a timestamp, a DNS-label author and valid initial state.
	if len(o.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if err := validateAuthor(o.GetAuthor()); err != nil {
		return err
	}
	for _, state := range o.GetInitialState() {
		if err := state.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *CreateChatChannelOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Validate the channel creation intent before changing World state.
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Initial events share the operation's author, timestamp, and transaction.
	objKey := o.GetObjectKey()
	var author *ChatResource
	if len(o.GetInitialState()) != 0 {
		author, err = newReplayResource(ctx, objKey, sender)
		if err != nil {
			return false, err
		}
	}

	// Prepare the channel metadata with an initialized thread index.
	channel := &ChatChannel{
		Name:                      o.GetName(),
		Topic:                     o.GetTopic(),
		CreatedAt:                 o.GetTimestamp(),
		CreatorPeerId:             sender.String(),
		CreatorAuthor:             o.GetAuthor(),
		EncryptionAlgorithm:       o.GetEncryptionAlgorithm(),
		ThreadIndexedMessageCount: new(uint64),
	}

	// Create the channel object with its initial metadata block.
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

	// Mark the channel object with its block type.
	if err := world_types.SetObjectType(ctx, ws, objKey, ChatChannelTypeID); err != nil {
		return false, err
	}

	// Append the initial state events with the channel creation author.
	for _, state := range o.GetInitialState() {
		request := &spacewave_chat_rpc.SendMessageRequest{
			Content: &ChatMessageContent{Content: &ChatMessageContent_StateChange{StateChange: state}},
			Author:  o.GetAuthor(),
		}
		if _, err := author.appendMessage(ctx, ws, request, o.GetTimestamp()); err != nil {
			return false, err
		}
	}

	return false, nil
}

var _ world.AuthenticatedOperation = (*CreateChatChannelOp)(nil)
