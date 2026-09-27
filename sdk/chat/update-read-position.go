package spacewave_chat

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// UpdateChatReadPositionOpID identifies a replayable chat receipt.
const UpdateChatReadPositionOpID = "spacewave-chat/read-position/update"

// AuthenticatedOperation selects the verified transaction signer on replay.
func (o *UpdateChatReadPositionOp) AuthenticatedOperation() {}

// GetOperationTypeId identifies the receipt operation for World replay.
func (o *UpdateChatReadPositionOp) GetOperationTypeId() string { return UpdateChatReadPositionOpID }

// Validate requires a channel and valid receipt timestamp.
func (o *UpdateChatReadPositionOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	return o.GetTimestamp().Validate(false)
}

// MarshalBlock serializes the receipt intent.
func (o *UpdateChatReadPositionOp) MarshalBlock() ([]byte, error) { return o.MarshalVT() }

// UnmarshalBlock restores the receipt intent.
func (o *UpdateChatReadPositionOp) UnmarshalBlock(data []byte) error { return o.UnmarshalVT(data) }

// ApplyWorldOp advances the accepted person's receipt in replay order.
func (o *UpdateChatReadPositionOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, sender peer.ID) (bool, error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	resource, err := newReplayResource(ctx, o.GetObjectKey(), sender)
	if err != nil {
		return false, err
	}
	return false, resource.applyReadPosition(ctx, ws, o.GetNextIndex(), o.GetTimestamp())
}

// ApplyWorldObjectOp rejects object-only replay for channel receipts.
func (o *UpdateChatReadPositionOp) ApplyWorldObjectOp(context.Context, *logrus.Entry, world.ObjectState, peer.ID) (bool, error) {
	return false, world.ErrUnhandledOp
}

// LookupUpdateChatReadPositionOp resolves the replayable receipt operation.
func LookupUpdateChatReadPositionOp(_ context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == UpdateChatReadPositionOpID {
		return &UpdateChatReadPositionOp{}, nil
	}
	return nil, nil
}

var _ world.AuthenticatedOperation = (*UpdateChatReadPositionOp)(nil)
