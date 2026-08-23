package spacewave_chat

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
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
	seen := make(map[string]struct{}, len(o.GetMemberPeerIds()))
	for _, memberID := range o.GetMemberPeerIds() {
		if memberID == "" {
			return errors.New("chat channel member peer id is empty")
		}
		if _, dup := seen[memberID]; dup {
			return errors.Errorf("chat channel member %s is listed more than once", memberID)
		}
		seen[memberID] = struct{}{}
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
		Name:          o.GetName(),
		Topic:         o.GetTopic(),
		CreatedAt:     o.GetTimestamp(),
		MemberPeerIds: o.GetMemberPeerIds(),
	}

	if _, _, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(channel, true)
		return nil
	}); err != nil {
		return false, err
	}

	if err := world_types.SetObjectType(ctx, ws, objKey, ChatChannelTypeID); err != nil {
		return false, err
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
