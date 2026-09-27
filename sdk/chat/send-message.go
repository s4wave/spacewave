package spacewave_chat

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// SendChatMessageOpID identifies a replayable channel append.
const SendChatMessageOpID = "spacewave-chat/message/send"

// GetOperationTypeId identifies the operation for World transaction replay.
func (o *SendChatMessageOp) GetOperationTypeId() string { return SendChatMessageOpID }

// AuthenticatedOperation selects the verified transaction signer on replay.
func (o *SendChatMessageOp) AuthenticatedOperation() {}

// Validate requires a stable send identity and timestamp.
func (o *SendChatMessageOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if o.GetRequest().GetTransactionId() == "" {
		return errors.New("chat append operation requires a transaction ID")
	}
	return o.GetTimestamp().Validate(false)
}

// MarshalBlock retains the append intent for World replay.
func (o *SendChatMessageOp) MarshalBlock() ([]byte, error) { return o.MarshalVT() }

// UnmarshalBlock restores the append intent without assigning a history index.
func (o *SendChatMessageOp) UnmarshalBlock(data []byte) error { return o.UnmarshalVT(data) }

// ApplyWorldOp appends against the World history accepted before this operation.
func (o *SendChatMessageOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, sender peer.ID) (bool, error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	resource, err := newReplayResource(ctx, o.GetObjectKey(), sender)
	if err != nil {
		return false, err
	}
	_, err = resource.appendMessage(ctx, ws, o.GetRequest(), o.GetTimestamp())
	return false, err
}

// ApplyWorldObjectOp rejects object-only application because an append spans World objects.
func (o *SendChatMessageOp) ApplyWorldObjectOp(context.Context, *logrus.Entry, world.ObjectState, peer.ID) (bool, error) {
	return false, world.ErrUnhandledOp
}

// LookupSendChatMessageOp resolves the replayable channel append operation.
func LookupSendChatMessageOp(_ context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == SendChatMessageOpID {
		return &SendChatMessageOp{}, nil
	}
	return nil, nil
}

// newReplayResource binds a replayed operation to its verified signing device
// and the person the replay authority resolved from the accepted config.
func newReplayResource(ctx context.Context, objectKey string, sender peer.ID) (*ChatResource, error) {
	person := world.OperationPersonFromContext(ctx)
	if sender == "" || person == "" {
		return nil, ErrChatAuthorIdentityRequired
	}
	return &ChatResource{objectKey: objectKey, localPeerID: sender.String(), device: sender, personID: person}, nil
}

// _ is a type assertion
var _ world.AuthenticatedOperation = (*SendChatMessageOp)(nil)
