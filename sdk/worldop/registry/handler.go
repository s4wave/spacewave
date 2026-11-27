package s4wave_worldop_registry

import (
	"context"

	"github.com/pkg/errors"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/sirupsen/logrus"
)

// Handler serves a plugin's World operations to the core registry. It decodes
// each operation with lookup and applies it to the World or object state the
// registry attaches to the call, under the sender the World authenticated. An
// OperationRejection returns to the applying client as its code and message.
type Handler struct {
	// le is the logger passed to each operation.
	le *logrus.Entry
	// lookup constructs an empty operation for a type ID.
	lookup world.LookupOp
}

// NewHandler constructs a Handler for the operations lookup knows.
func NewHandler(le *logrus.Entry, lookup world.LookupOp) *Handler {
	return &Handler{le: le, lookup: lookup}
}

// ApplyWorldOp applies an operation to the attached World state.
func (h *Handler) ApplyWorldOp(ctx context.Context, req *ApplyWorldOpRequest) (*ApplyWorldOpResponse, error) {
	// Decode the operation and its sender.
	op, sender, err := h.decode(ctx, req.GetOperationTypeId(), req.GetOpData(), req.GetSender())
	if err != nil {
		return nil, err
	}

	// Open the attached World state.
	if req.GetAttachedWorldStateResourceId() == 0 {
		return nil, errors.New("attached World state resource is required")
	}
	client, err := attachedClient(ctx)
	if err != nil {
		return nil, err
	}
	ref := client.CreateResourceReference(req.GetAttachedWorldStateResourceId())
	ws, err := sdk_world_engine.NewSDKWorldState(client, ref, false)
	if err != nil {
		ref.Release()
		return nil, err
	}
	defer ws.Release()

	// Apply the operation.
	sysErr, err := op.ApplyWorldOp(ctx, h.le, ws, sender)
	code, msg, err := splitRejection(err)
	if err != nil {
		return nil, err
	}
	return &ApplyWorldOpResponse{SystemError: sysErr, RejectionCode: code, RejectionMessage: msg}, nil
}

// ApplyWorldObjectOp applies an operation to the attached object state, which
// must be the object the request references.
func (h *Handler) ApplyWorldObjectOp(ctx context.Context, req *ApplyWorldObjectOpRequest) (*ApplyWorldObjectOpResponse, error) {
	// Decode the operation and its sender.
	op, sender, err := h.decode(ctx, req.GetOperationTypeId(), req.GetOpData(), req.GetSender())
	if err != nil {
		return nil, err
	}

	// Reference the attached object and check its key.
	if req.GetAttachedObjectStateResourceId() == 0 {
		return nil, errors.New("attached object state resource is required")
	}
	client, err := attachedClient(ctx)
	if err != nil {
		return nil, err
	}
	ref := client.CreateResourceReference(req.GetAttachedObjectStateResourceId())
	objectKey, err := attachedObjectKey(ctx, ref)
	if err == nil && objectKey != req.GetObjectKey() {
		err = errors.Errorf("attached object %q is not %q", objectKey, req.GetObjectKey())
	}
	if err != nil {
		ref.Release()
		return nil, err
	}

	// Open the object and apply the operation.
	os, err := sdk_world_engine.NewSDKObjectState(client, ref, objectKey, false)
	if err != nil {
		ref.Release()
		return nil, err
	}
	defer os.Release()
	sysErr, err := world.ApplyWorldObjectOp(ctx, h.le, op, os, sender)
	code, msg, err := splitRejection(err)
	if err != nil {
		return nil, err
	}
	return &ApplyWorldObjectOpResponse{SystemError: sysErr, RejectionCode: code, RejectionMessage: msg}, nil
}

// ValidateOp checks operation bytes before the host supplies World state.
func (h *Handler) ValidateOp(ctx context.Context, req *ValidateOpRequest) (*ValidateOpResponse, error) {
	op, _, err := h.decode(ctx, req.GetOperationTypeId(), req.GetOpData(), "")
	if err == nil {
		err = world.ValidateOp(op)
	}
	if err != nil {
		return &ValidateOpResponse{Error: err.Error()}, nil
	}
	return &ValidateOpResponse{}, nil
}

// decode constructs the operation of type opTypeID from data, and decodes the
// base58 sender, which is empty when the World supplies none.
func (h *Handler) decode(ctx context.Context, opTypeID string, data []byte, encodedSender string) (world.Operation, peer.ID, error) {
	// Construct the operation, then decode its data.
	op, err := h.lookup(ctx, opTypeID)
	if err == nil && op == nil {
		err = world.ErrUnhandledOp
	}
	if err == nil {
		err = op.UnmarshalVT(data)
	}
	if err != nil {
		return nil, "", errors.Wrapf(err, "operation %s", opTypeID)
	}

	// Decode the sender.
	if encodedSender == "" {
		return op, "", nil
	}
	sender, err := peer.IDB58Decode(encodedSender)
	if err != nil {
		return nil, "", errors.Wrap(err, "decode operation sender")
	}
	return op, sender, nil
}

// attachedClient returns the client for resources attached to the call.
func attachedClient(ctx context.Context) (*sdk_world_engine.AttachedResourceClient, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	return sdk_world_engine.NewAttachedResourceClient(resourceCtx), nil
}

// attachedObjectKey reads the key of the attached object state.
func attachedObjectKey(ctx context.Context, ref resource_client.ResourceRef) (string, error) {
	// Ask the object state resource for its key.
	client, err := ref.GetClient()
	if err != nil {
		return "", err
	}
	resp, err := s4wave_world.NewSRPCObjectStateResourceServiceClient(client).GetKey(ctx, &s4wave_world.GetKeyRequest{})
	if err != nil {
		return "", err
	}
	return resp.GetObjectKey(), nil
}

// splitRejection separates an OperationRejection from other errors.
func splitRejection(err error) (code, msg string, rest error) {
	var rejection *world.OperationRejection
	if errors.As(err, &rejection) {
		return rejection.Code, rejection.Message, nil
	}
	return "", "", err
}

// _ is a type assertion
var _ SRPCWorldOpHandlerServiceServer = (*Handler)(nil)
