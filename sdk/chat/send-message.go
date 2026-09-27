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

// Validate requires stable send identity, attribution, and timestamp.
func (o *SendChatMessageOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if o.GetRequest().GetTransactionId() == "" {
		return errors.New("chat append operation requires a transaction ID")
	}
	if o.GetSenderPeerId() == "" || o.GetPersonPeerId() == "" {
		return ErrChatAuthorIdentityRequired
	}
	return o.GetTimestamp().Validate(false)
}

// MarshalBlock retains the append intent for World replay.
func (o *SendChatMessageOp) MarshalBlock() ([]byte, error) { return o.MarshalVT() }

// UnmarshalBlock restores the append intent without assigning a history index.
func (o *SendChatMessageOp) UnmarshalBlock(data []byte) error { return o.UnmarshalVT(data) }

// ApplyWorldOp appends against the World history accepted before this operation.
func (o *SendChatMessageOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, _ peer.ID) (bool, error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	resource := &ChatResource{objectKey: o.GetObjectKey(), localPeerID: o.GetSenderPeerId(), personPeerID: o.GetPersonPeerId()}
	_, err := resource.appendMessage(ctx, ws, o.GetRequest(), o.GetTimestamp())
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

// _ is a type assertion
var _ world.Operation = (*SendChatMessageOp)(nil)
