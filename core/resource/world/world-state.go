package resource_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_bucket_lookup "github.com/s4wave/spacewave/core/resource/bucket/lookup"
	"github.com/s4wave/spacewave/db/block/quad"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// WorldStateResource wraps a WorldState for resource access.
type WorldStateResource struct {
	le       *logrus.Entry
	b        bus.Bus
	mux      srpc.Invoker
	ws       world.WorldState
	lookupOp world.LookupOp
}

// NewWorldStateResource creates a new WorldStateResource.
//
// lookupOp may be nil
func NewWorldStateResource(le *logrus.Entry, b bus.Bus, ws world.WorldState, lookupOp world.LookupOp) *WorldStateResource {
	wsResource := &WorldStateResource{le: le, b: b, ws: ws, lookupOp: lookupOp}
	mux := srpc.NewMux()
	_ = s4wave_world.SRPCRegisterWorldStateResourceService(mux, wsResource)
	// Note: TypedObjectResource is not registered here because it requires an Engine
	// for write operations. Use EngineResource.AccessTypedObject instead.
	wsResource.mux = mux
	return wsResource
}

// GetMux returns the rpc mux.
func (r *WorldStateResource) GetMux() srpc.Invoker {
	return r.mux
}

// GetReadOnly returns if the world state is read-only.
func (r *WorldStateResource) GetReadOnly(ctx context.Context, req *s4wave_world.GetReadOnlyRequest) (*s4wave_world.GetReadOnlyResponse, error) {
	return &s4wave_world.GetReadOnlyResponse{ReadOnly: r.ws.GetReadOnly()}, nil
}

// GetSeqno returns the current seqno of the world state.
func (r *WorldStateResource) GetSeqno(ctx context.Context, req *s4wave_world.GetSeqnoRequest) (*s4wave_world.GetSeqnoResponse, error) {
	seqno, err := r.ws.GetSeqno(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_world.GetSeqnoResponse{Seqno: seqno}, nil
}

// WaitSeqno waits for the seqno of the world state to be >= value.
func (r *WorldStateResource) WaitSeqno(ctx context.Context, req *s4wave_world.WaitSeqnoRequest) (*s4wave_world.WaitSeqnoResponse, error) {
	seqno, err := r.ws.WaitSeqno(ctx, req.GetSeqno())
	if err != nil {
		return nil, err
	}
	return &s4wave_world.WaitSeqnoResponse{Seqno: seqno}, nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
func (r *WorldStateResource) BuildStorageCursor(ctx context.Context, req *s4wave_world.BuildStorageCursorRequest) (*s4wave_world.BuildStorageCursorResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	cursor, err := r.ws.BuildStorageCursor(ctx)
	if err != nil {
		return nil, err
	}

	cursorResource := resource_bucket_lookup.NewBucketLookupCursorResource(r.le, r.b, cursor)
	id, err := resourceCtx.AddResource(cursorResource.GetMux(), func() {
		cursor.Release()
	})
	if err != nil {
		cursor.Release()
		return nil, err
	}

	return &s4wave_world.BuildStorageCursorResponse{ResourceId: id}, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
func (r *WorldStateResource) AccessWorldState(ctx context.Context, req *s4wave_world.AccessWorldStateRequest) (*s4wave_world.AccessWorldStateResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var cursorResource *resource_bucket_lookup.BucketLookupCursorResource
	err = r.ws.AccessWorldState(ctx, req.GetRef(), func(c *bucket_lookup.Cursor) error {
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

// CreateObject creates an object with a key and initial root ref.
func (r *WorldStateResource) CreateObject(ctx context.Context, req *s4wave_world.CreateObjectRequest) (*s4wave_world.CreateObjectResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	obj, err := r.ws.CreateObject(ctx, req.GetObjectKey(), req.GetRootRef())
	if err != nil {
		return nil, err
	}

	objResource := NewObjectStateResource(r.le, r.b, obj, r.lookupOp)
	id, err := resourceCtx.AddResource(objResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_world.CreateObjectResponse{ResourceId: id, ObjectKey: obj.GetKey()}, nil
}

// GetObject looks up an object by key.
func (r *WorldStateResource) GetObject(ctx context.Context, req *s4wave_world.GetObjectRequest) (*s4wave_world.GetObjectResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	obj, found, err := r.ws.GetObject(ctx, req.GetObjectKey())
	if err != nil {
		return nil, err
	}

	if !found {
		return &s4wave_world.GetObjectResponse{Found: false}, nil
	}

	objResource := NewObjectStateResource(r.le, r.b, obj, r.lookupOp)
	id, err := resourceCtx.AddResource(objResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_world.GetObjectResponse{Found: true, ResourceId: id, ObjectKey: obj.GetKey()}, nil
}

// IterateObjects returns an iterator with the given object key prefix.
func (r *WorldStateResource) IterateObjects(ctx context.Context, req *s4wave_world.IterateObjectsRequest) (*s4wave_world.IterateObjectsResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	iter := r.ws.IterateObjects(ctx, req.GetPrefix(), req.GetReversed())
	iterResource := NewObjectIteratorResource(r.le, r.b, iter)
	id, err := resourceCtx.AddResource(iterResource.GetMux(), func() {
		iter.Close()
	})
	if err != nil {
		iter.Close()
		return nil, err
	}

	return &s4wave_world.IterateObjectsResponse{ResourceId: id}, nil
}

// RenameObject renames an object key and associated graph quads.
func (r *WorldStateResource) RenameObject(ctx context.Context, req *s4wave_world.RenameObjectRequest) (*s4wave_world.RenameObjectResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	obj, err := r.ws.RenameObject(ctx, req.GetOldObjectKey(), req.GetNewObjectKey(), req.GetDescendants())
	if err != nil {
		return nil, err
	}

	objResource := NewObjectStateResource(r.le, r.b, obj, r.lookupOp)
	id, err := resourceCtx.AddResource(objResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_world.RenameObjectResponse{ResourceId: id, ObjectKey: obj.GetKey()}, nil
}

// DeleteObject deletes an object and associated graph quads by ID.
func (r *WorldStateResource) DeleteObject(ctx context.Context, req *s4wave_world.DeleteObjectRequest) (*s4wave_world.DeleteObjectResponse, error) {
	deleted, err := r.ws.DeleteObject(ctx, req.GetObjectKey())
	if err != nil {
		return nil, err
	}
	return &s4wave_world.DeleteObjectResponse{Deleted: deleted}, nil
}

// SetGraphQuad sets a quad in the graph store.
func (r *WorldStateResource) SetGraphQuad(ctx context.Context, req *s4wave_world.SetGraphQuadRequest) (*s4wave_world.SetGraphQuadResponse, error) {
	q := req.GetQuad()
	gq := world.NewGraphQuad(q.GetSubject(), q.GetPredicate(), q.GetObj(), q.GetLabel())
	err := r.ws.SetGraphQuad(ctx, gq)
	if err != nil {
		return nil, err
	}
	return &s4wave_world.SetGraphQuadResponse{}, nil
}

// DeleteGraphQuad deletes a quad from the graph store.
func (r *WorldStateResource) DeleteGraphQuad(ctx context.Context, req *s4wave_world.DeleteGraphQuadRequest) (*s4wave_world.DeleteGraphQuadResponse, error) {
	q := req.GetQuad()
	gq := world.NewGraphQuad(q.GetSubject(), q.GetPredicate(), q.GetObj(), q.GetLabel())
	err := r.ws.DeleteGraphQuad(ctx, gq)
	if err != nil {
		return nil, err
	}
	return &s4wave_world.DeleteGraphQuadResponse{}, nil
}

// LookupGraphQuads searches for graph quads in the store.
func (r *WorldStateResource) LookupGraphQuads(ctx context.Context, req *s4wave_world.LookupGraphQuadsRequest) (*s4wave_world.LookupGraphQuadsResponse, error) {
	f := req.GetFilter()
	filter := world.NewGraphQuad(f.GetSubject(), f.GetPredicate(), f.GetObj(), f.GetLabel())
	quads, err := r.ws.LookupGraphQuads(ctx, filter, req.GetLimit())
	if err != nil {
		return nil, err
	}

	protoQuads := make([]*quad.Quad, len(quads))
	for i, q := range quads {
		protoQuads[i] = &quad.Quad{
			Subject:   q.GetSubject(),
			Predicate: q.GetPredicate(),
			Obj:       q.GetObj(),
			Label:     q.GetLabel(),
		}
	}

	return &s4wave_world.LookupGraphQuadsResponse{Quads: protoQuads}, nil
}

// ListObjectsWithType lists object keys with the given type identifier.
func (r *WorldStateResource) ListObjectsWithType(ctx context.Context, req *s4wave_world.ListObjectsWithTypeRequest) (*s4wave_world.ListObjectsWithTypeResponse, error) {
	objKeys, err := world_types.ListObjectsWithType(ctx, r.ws, req.GetTypeId())
	if err != nil {
		return nil, err
	}
	return &s4wave_world.ListObjectsWithTypeResponse{ObjectKeys: objKeys}, nil
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
func (r *WorldStateResource) DeleteGraphObject(ctx context.Context, req *s4wave_world.DeleteGraphObjectRequest) (*s4wave_world.DeleteGraphObjectResponse, error) {
	err := r.ws.DeleteGraphObject(ctx, req.GetObjectKey())
	if err != nil {
		return nil, err
	}
	return &s4wave_world.DeleteGraphObjectResponse{}, nil
}

// ApplyWorldOp applies a batch operation at the world level.
func (r *WorldStateResource) ApplyWorldOp(ctx context.Context, req *s4wave_world.ApplyWorldOpRequest) (*s4wave_world.ApplyWorldOpResponse, error) {
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

	seqno, sysErr, err := r.ws.ApplyWorldOp(ctx, op, opSender)
	if err != nil {
		return nil, err
	}

	return &s4wave_world.ApplyWorldOpResponse{Seqno: seqno, SysErr: sysErr}, nil
}

// _ is a type assertion
var _ s4wave_world.SRPCWorldStateResourceServiceServer = ((*WorldStateResource)(nil))
