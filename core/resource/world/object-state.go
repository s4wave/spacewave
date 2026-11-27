package resource_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// ObjectStateResource wraps an ObjectState for resource access.
type ObjectStateResource struct {
	le                 *logrus.Entry
	b                  bus.Bus
	mux                srpc.Invoker
	obj                world.ObjectState
	lookupOp           world.LookupOp
	sessionPeerID      peer.ID
	sessionPeerIDBound bool
}

// NewObjectStateResource creates a new ObjectStateResource.
//
// It borrows obj for the resource lifetime. The caller must release obj after
// retiring the resource. lookupOp may be nil.
func NewObjectStateResource(le *logrus.Entry, b bus.Bus, obj world.ObjectState, lookupOp world.LookupOp, opts ...WorldStateResourceOption) *ObjectStateResource {
	// Construct the resource and register its service on the rpc mux.
	objResource := &ObjectStateResource{le: le, b: b, obj: obj, lookupOp: lookupOp}
	objResource.sessionPeerID, objResource.sessionPeerIDBound = worldStateResourceSessionPeerID(opts...)
	mux := srpc.NewMux()
	_ = s4wave_world.SRPCRegisterObjectStateResourceService(mux, objResource)
	objResource.mux = mux
	return objResource
}

// GetMux returns the rpc mux.
func (r *ObjectStateResource) GetMux() srpc.Invoker {
	return r.mux
}

// GetRootRef returns the root reference of the object.
func (r *ObjectStateResource) GetRootRef(ctx context.Context, req *s4wave_world.GetRootRefRequest) (*s4wave_world.GetRootRefResponse, error) {
	// Read the root reference and revision from the object.
	ref, rev, err := r.obj.GetRootRef(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_world.GetRootRefResponse{RootRef: ref, Rev: rev}, nil
}

// GetModSeqno returns the World seqno after the last commit that changed the
// object, or zero when its World cannot report it.
func (r *ObjectStateResource) GetModSeqno(ctx context.Context, _ *s4wave_world.GetModSeqnoRequest) (*s4wave_world.GetModSeqnoResponse, error) {
	seqno, err := r.obj.GetModSeqno(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_world.GetModSeqnoResponse{ModSeqno: seqno}, nil
}

// SetRootRef updates the root reference of the object. A write the World
// refuses before its turn opens returns as its code to a client that decodes
// it, and as the call's error otherwise.
func (r *ObjectStateResource) SetRootRef(ctx context.Context, req *s4wave_world.SetRootRefRequest) (*s4wave_world.SetRootRefResponse, error) {
	// Write the new root reference and read back the revision.
	rev, err := r.obj.SetRootRef(ctx, req.GetRootRef())
	code, err := writeErrorCode(req.GetKnownErrorCode(), err)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return &s4wave_world.SetRootRefResponse{ErrorCode: code}, nil
	}
	return &s4wave_world.SetRootRefResponse{Rev: rev}, nil
}

// GetKey returns the key identifier of the object.
func (r *ObjectStateResource) GetKey(ctx context.Context, req *s4wave_world.GetKeyRequest) (*s4wave_world.GetKeyResponse, error) {
	return &s4wave_world.GetKeyResponse{ObjectKey: r.obj.GetKey()}, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
func (r *ObjectStateResource) AccessWorldState(ctx context.Context, req *s4wave_world.AccessWorldStateRequest) (*s4wave_world.AccessWorldStateResponse, error) {
	// Resolve the resource client context for the caller.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Register the cursor while its AccessWorldState callback stays open.
	id, err := addAccessWorldStateResource(ctx, resourceCtx, r.le, r.b, func(ctx context.Context, cb func(*bucket_lookup.Cursor) error) error {
		return r.obj.AccessWorldState(ctx, req.GetRef(), cb)
	})
	if err != nil {
		return nil, err
	}

	return &s4wave_world.AccessWorldStateResponse{ResourceId: id}, nil
}

// ApplyObjectOp applies a batch operation at the object level.
// A refusal whose code the client does not decode returns as the call's error.
func (r *ObjectStateResource) ApplyObjectOp(ctx context.Context, req *s4wave_world.ApplyObjectOpRequest) (*s4wave_world.ApplyObjectOpResponse, error) {
	resp, err := r.applyObjectOp(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := undecodedRefusal(req.GetKnownErrorCode(), resp.GetErrorCode()); err != nil {
		return nil, err
	}
	return resp, nil
}

// applyObjectOp resolves and applies the operation req selects to the object.
func (r *ObjectStateResource) applyObjectOp(ctx context.Context, req *s4wave_world.ApplyObjectOpRequest) (*s4wave_world.ApplyObjectOpResponse, error) {
	// Reject the operation when no lookup op is configured.
	if r.lookupOp == nil {
		return &s4wave_world.ApplyObjectOpResponse{
			ErrorCode: s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP,
		}, nil
	}

	// Resolve the operation implementation for the requested type.
	op, err := r.lookupOp(ctx, req.GetOpTypeId())
	if err == nil && op == nil {
		err = world.ErrUnhandledOp
	}
	if err != nil {
		if errors.Is(err, world.ErrUnhandledOp) {
			return &s4wave_world.ApplyObjectOpResponse{
				ErrorCode: s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP,
			}, nil
		}
		return nil, err
	}

	// Decode the operation payload into the resolved operation.
	err = op.UnmarshalVT(req.GetOpData())
	if err != nil {
		return nil, err
	}

	// Determine the sender peer ID, falling back to the request.
	opSender := r.sessionPeerID
	if !r.sessionPeerIDBound {
		opSender, err = req.ParsePeerID()
		if err != nil {
			return nil, err
		}
	}

	// Apply the operation to the object and map rejections, and a plugin
	// timeout when the discarded transaction kept nothing, or an unavailable
	// plugin, which was never sent the operation.
	rev, sysErr, err := r.obj.ApplyObjectOp(ctx, op, opSender)
	if err != nil {
		code, rejection, ok := opErrorRefusal(err, world.AppliesAtomically(r.obj))
		if !ok {
			return nil, err
		}
		resp := &s4wave_world.ApplyObjectOpResponse{ErrorCode: code}
		if rejection != nil {
			resp.RejectionCode, resp.RejectionMessage = rejection.Code, rejection.Message
		}
		return resp, nil
	}

	return &s4wave_world.ApplyObjectOpResponse{Rev: rev, SysErr: sysErr}, nil
}

// IncrementRev increments the revision of the object.
func (r *ObjectStateResource) IncrementRev(ctx context.Context, req *s4wave_world.IncrementRevRequest) (*s4wave_world.IncrementRevResponse, error) {
	// Increment the revision and report a replay wait as its code.
	rev, err := r.obj.IncrementRev(ctx)
	code, err := writeErrorCode(req.GetKnownErrorCode(), err)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return &s4wave_world.IncrementRevResponse{ErrorCode: code}, nil
	}

	// Return the revision after the increment.
	return &s4wave_world.IncrementRevResponse{Rev: rev}, nil
}

// WaitRev waits until the object rev is >= the specified revision.
func (r *ObjectStateResource) WaitRev(ctx context.Context, req *s4wave_world.WaitRevRequest) (*s4wave_world.WaitRevResponse, error) {
	rev, err := r.obj.WaitRev(ctx, req.GetRev(), req.GetIgnoreNotFound())
	if err != nil {
		return nil, err
	}
	return &s4wave_world.WaitRevResponse{Rev: rev}, nil
}

// _ is a type assertion
var _ s4wave_world.SRPCObjectStateResourceServiceServer = (*ObjectStateResource)(nil)
