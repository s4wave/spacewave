package resource_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_bucket_lookup "github.com/s4wave/spacewave/core/resource/bucket/lookup"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// ObjectStateResource wraps an ObjectState for resource access.
type ObjectStateResource struct {
	le       *logrus.Entry
	b        bus.Bus
	mux      srpc.Invoker
	obj      world.ObjectState
	lookupOp world.LookupOp
}

// NewObjectStateResource creates a new ObjectStateResource.
//
// lookupOp may be nil
func NewObjectStateResource(le *logrus.Entry, b bus.Bus, obj world.ObjectState, lookupOp world.LookupOp) *ObjectStateResource {
	objResource := &ObjectStateResource{le: le, b: b, obj: obj, lookupOp: lookupOp}
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
	ref, rev, err := r.obj.GetRootRef(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_world.GetRootRefResponse{RootRef: ref, Rev: rev}, nil
}

// SetRootRef updates the root reference of the object.
func (r *ObjectStateResource) SetRootRef(ctx context.Context, req *s4wave_world.SetRootRefRequest) (*s4wave_world.SetRootRefResponse, error) {
	rev, err := r.obj.SetRootRef(ctx, req.GetRootRef())
	if err != nil {
		return nil, err
	}
	return &s4wave_world.SetRootRefResponse{Rev: rev}, nil
}

// GetKey returns the key identifier of the object.
func (r *ObjectStateResource) GetKey(ctx context.Context, req *s4wave_world.GetKeyRequest) (*s4wave_world.GetKeyResponse, error) {
	return &s4wave_world.GetKeyResponse{ObjectKey: r.obj.GetKey()}, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
func (r *ObjectStateResource) AccessWorldState(ctx context.Context, req *s4wave_world.AccessWorldStateRequest) (*s4wave_world.AccessWorldStateResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var cursorResource *resource_bucket_lookup.BucketLookupCursorResource
	err = r.obj.AccessWorldState(ctx, req.GetRef(), func(c *bucket_lookup.Cursor) error {
		cursorResource = resource_bucket_lookup.NewBucketLookupCursorResource(r.le, r.b, c)
		return nil
	})
	if err != nil {
		return nil, err
	}

	id, err := resourceCtx.AddResource(cursorResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_world.AccessWorldStateResponse{ResourceId: id}, nil
}

// ApplyObjectOp applies a batch operation at the object level.
func (r *ObjectStateResource) ApplyObjectOp(ctx context.Context, req *s4wave_world.ApplyObjectOpRequest) (*s4wave_world.ApplyObjectOpResponse, error) {
	if r.lookupOp == nil {
		return nil, world.ErrUnhandledOp
	}

	op, err := r.lookupOp(ctx, req.GetOpTypeId())
	if err == nil && op == nil {
		err = world.ErrUnhandledOp
	}
	if err != nil {
		return nil, err
	}

	err = op.UnmarshalBlock(req.GetOpData())
	if err != nil {
		return nil, err
	}

	opSender, err := req.ParsePeerID()
	if err != nil {
		return nil, err
	}

	rev, sysErr, err := r.obj.ApplyObjectOp(ctx, op, opSender)
	if err != nil {
		return nil, err
	}

	return &s4wave_world.ApplyObjectOpResponse{Rev: rev, SysErr: sysErr}, nil
}

// IncrementRev increments the revision of the object.
func (r *ObjectStateResource) IncrementRev(ctx context.Context, req *s4wave_world.IncrementRevRequest) (*s4wave_world.IncrementRevResponse, error) {
	rev, err := r.obj.IncrementRev(ctx)
	if err != nil {
		return nil, err
	}
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
var _ s4wave_world.SRPCObjectStateResourceServiceServer = ((*ObjectStateResource)(nil))
