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

// WorldStageResource serves cursors whose writes a World stage owns.
// Releasing the resource releases the stage.
type WorldStageResource struct {
	// le logs cursor resource failures.
	le *logrus.Entry
	// b resolves cursor directives.
	b bus.Bus
	// stage owns the writes made through this resource's cursors.
	stage world.WorldStage
	// mux serves the stage RPCs.
	mux srpc.Invoker
}

// NewWorldStageResource wraps a World stage for resource access.
func NewWorldStageResource(le *logrus.Entry, b bus.Bus, stage world.WorldStage) *WorldStageResource {
	r := &WorldStageResource{le: le, b: b, stage: stage}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_world.SRPCRegisterWorldStageResourceService(mux, r)
	})
	return r
}

// GetMux returns the rpc mux.
func (r *WorldStageResource) GetMux() srpc.Invoker {
	return r.mux
}

// BuildStorageCursor builds a staged cursor to the world storage.
func (r *WorldStageResource) BuildStorageCursor(ctx context.Context, req *s4wave_world.BuildStorageCursorRequest) (*s4wave_world.BuildStorageCursorResponse, error) {
	// Acquire the resource client and build a staged cursor.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	cursor, err := r.stage.BuildStorageCursor(ctx)
	if err != nil {
		return nil, err
	}

	// Register the cursor resource with its release hook.
	cursorResource := resource_bucket_lookup.NewBucketLookupCursorResource(r.le, r.b, cursor)
	id, err := resourceCtx.AddResource(cursorResource.GetMux(), cursor.Release)
	if err != nil {
		cursor.Release()
		return nil, err
	}
	return &s4wave_world.BuildStorageCursorResponse{ResourceId: id}, nil
}

// AccessWorldState builds a staged cursor with an optional ref.
func (r *WorldStageResource) AccessWorldState(ctx context.Context, req *s4wave_world.AccessWorldStateRequest) (*s4wave_world.AccessWorldStateResponse, error) {
	// Acquire the resource client.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Register the cursor while its AccessWorldState callback stays open.
	id, err := addAccessWorldStateResource(ctx, resourceCtx, r.le, r.b, func(ctx context.Context, cb func(*bucket_lookup.Cursor) error) error {
		return r.stage.AccessWorldState(ctx, req.GetRef(), cb)
	})
	if err != nil {
		return nil, err
	}
	return &s4wave_world.AccessWorldStateResponse{ResourceId: id}, nil
}

// addWorldStageResource opens a stage with open and registers it with the
// resource client of ctx. Releasing the resource, or ending the client,
// releases the stage.
func addWorldStageResource(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	open func(context.Context) (world.WorldStage, error),
) (*s4wave_world.StageWorldStateResponse, error) {
	// Acquire the resource client and open the stage.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	stage, err := open(ctx)
	if err != nil {
		return nil, err
	}

	// Release the stage with its resource or resource client.
	stageResource := NewWorldStageResource(le, b, stage)
	id, err := resourceCtx.AddResource(stageResource.GetMux(), stage.Release)
	if err != nil {
		stage.Release()
		return nil, err
	}
	return &s4wave_world.StageWorldStateResponse{ResourceId: id}, nil
}

// _ is a type assertion
var _ s4wave_world.SRPCWorldStageResourceServiceServer = (*WorldStageResource)(nil)
