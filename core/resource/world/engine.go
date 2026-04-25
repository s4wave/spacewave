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

// EngineResource wraps an Engine for resource access.
type EngineResource struct {
	le         *logrus.Entry
	b          bus.Bus
	mux        srpc.Invoker
	engine     world.Engine
	lookupOp   world.LookupOp
	engineInfo *s4wave_world.EngineInfo
}

// NewEngineResource creates a new EngineResource.
func NewEngineResource(le *logrus.Entry, b bus.Bus, w world.Engine, lookupOp world.LookupOp, engineInfo *s4wave_world.EngineInfo) *EngineResource {
	engineResource := &EngineResource{
		le:         le,
		b:          b,
		engine:     w,
		lookupOp:   lookupOp,
		engineInfo: engineInfo,
	}
	engineResource.mux = resource_server.NewResourceMux(
		func(mux srpc.Mux) error { return s4wave_world.SRPCRegisterEngineResourceService(mux, engineResource) },
		func(mux srpc.Mux) error {
			return s4wave_world.SRPCRegisterWatchWorldStateResourceService(mux, engineResource)
		},
		func(mux srpc.Mux) error {
			return s4wave_world.SRPCRegisterTypedObjectResourceService(mux, engineResource)
		},
	)
	return engineResource
}

// GetMux returns the rpc mux.
func (r *EngineResource) GetMux() srpc.Invoker {
	return r.mux
}

// GetEngineInfo returns information about the world engine.
func (r *EngineResource) GetEngineInfo(ctx context.Context, req *s4wave_world.GetEngineInfoRequest) (*s4wave_world.GetEngineInfoResponse, error) {
	return &s4wave_world.GetEngineInfoResponse{EngineInfo: r.engineInfo}, nil
}

// GetSeqno returns the current seqno of the world state.
func (r *EngineResource) GetSeqno(ctx context.Context, req *s4wave_world.GetSeqnoRequest) (*s4wave_world.GetSeqnoResponse, error) {
	wtx, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer wtx.Discard()

	seqno, err := wtx.GetSeqno(ctx)
	if err != nil {
		return nil, err
	}

	return &s4wave_world.GetSeqnoResponse{Seqno: seqno}, nil
}

// WaitSeqno waits for the seqno of the world state to be >= value.
func (r *EngineResource) WaitSeqno(ctx context.Context, req *s4wave_world.WaitSeqnoRequest) (*s4wave_world.WaitSeqnoResponse, error) {
	seqno, err := r.engine.WaitSeqno(ctx, req.GetSeqno())
	if err != nil {
		return nil, err
	}
	return &s4wave_world.WaitSeqnoResponse{Seqno: seqno}, nil
}

// NewTransaction creates a new transaction against the world state.
func (r *EngineResource) NewTransaction(ctx context.Context, req *s4wave_world.NewTransactionRequest) (*s4wave_world.NewTransactionResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	wtx, err := r.engine.NewTransaction(ctx, req.GetWrite())
	if err != nil {
		return nil, err
	}

	txResource := NewTxResource(r.le, r.b, wtx, r.lookupOp, r.engine)
	id, err := resourceCtx.AddResource(txResource.GetMux(), func() {
		wtx.Discard()
	})
	if err != nil {
		wtx.Discard()
		return nil, err
	}

	return &s4wave_world.NewTransactionResponse{ResourceId: id}, nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
func (r *EngineResource) BuildStorageCursor(ctx context.Context, req *s4wave_world.BuildStorageCursorRequest) (*s4wave_world.BuildStorageCursorResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	cursor, err := r.engine.BuildStorageCursor(ctx)
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
func (r *EngineResource) AccessWorldState(ctx context.Context, req *s4wave_world.AccessWorldStateRequest) (*s4wave_world.AccessWorldStateResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var cursorResource *resource_bucket_lookup.BucketLookupCursorResource
	err = r.engine.AccessWorldState(ctx, req.GetRef(), func(c *bucket_lookup.Cursor) error {
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

// WatchWorldState implements the streaming watch RPC.
// Change detection starts immediately as client accesses resources.
func (r *EngineResource) WatchWorldState(
	req *s4wave_world.WatchWorldStateRequest,
	stream s4wave_world.SRPCWatchWorldStateResourceService_WatchWorldStateStream,
) error {
	ctx := stream.Context()
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return err
	}

	for {
		// Create a read transaction to get current world state
		wtx, err := r.engine.NewTransaction(ctx, false)
		if err != nil {
			return err
		}

		// Get current world seqno
		seqno, err := wtx.GetSeqno(ctx)
		if err != nil {
			wtx.Discard()
			return err
		}

		// Create new tracked WorldState (empty - no tracking yet)
		// StateRoutineContainer starts immediately with empty snapshot
		trackedWs := NewTrackedWorldState(wtx, seqno, ctx)

		// Register as a resource
		trackedResource := NewWorldStateResource(r.le, r.b, trackedWs, r.lookupOp)
		resourceId, err := resourceCtx.AddResource(trackedResource.GetMux(), func() {
			trackedWs.Close()
		})
		if err != nil {
			trackedWs.Close()
			return err
		}

		// Send resource_id to client
		err = stream.Send(&s4wave_world.WatchWorldStateResponse{
			ResourceId: resourceId,
		})
		if err != nil {
			resourceCtx.ReleaseResource(resourceId)
			return err
		}

		// Wait for tracked resources to change
		// As client calls methods on TrackedWorldState:
		//   1. Access recorded (e.g., GetObject called)
		//   2. Snapshot cloned and updated
		//   3. SetState() called on StateRoutineContainer
		//   4. StateRoutineContainer compares snapshots (EqualVT)
		//   5. If different, restarts watchTrackedChanges with new snapshot
		//   6. watchTrackedChanges checks resources, waits on world seqno
		//   7. When change detected, returns nil, writes to changeResultCh
		// WaitForChanges blocks on changeResultCh until change or error
		err = trackedWs.WaitForChanges(ctx)

		// Release the tracked resource
		_ = resourceCtx.ReleaseResource(resourceId)

		if err != nil {
			// Context canceled or other error
			return err
		}

		// Changes detected - loop will create new tracked WorldState (empty)
	}
}

// AccessTypedObject looks up an object, determines its type, and returns a typed resource.
func (r *EngineResource) AccessTypedObject(ctx context.Context, req *s4wave_world.AccessTypedObjectRequest) (*s4wave_world.AccessTypedObjectResponse, error) {
	// Use an engine-backed WorldState so that each internal operation creates its
	// own short-lived transaction. This avoids two problems with the previous
	// approach of creating a single read-only Tx here:
	//  1. The Tx was read-only, so typed resources (e.g. UnixFS) were created
	//     without a writer, making file uploads fail with "read-only fs".
	//  2. The Tx was discarded after this method returned, but the created
	//     resource (e.g. FSCursor) outlived it and used it for lazy operations.
	ws := world.NewEngineWorldState(r.engine, true)
	typedResource := NewTypedObjectResource(r.le, r.b, ws, r.engine)
	return typedResource.AccessTypedObject(ctx, req)
}

// _ is a type assertion
var (
	_ s4wave_world.SRPCEngineResourceServiceServer          = (*EngineResource)(nil)
	_ s4wave_world.SRPCWatchWorldStateResourceServiceServer = (*EngineResource)(nil)
	_ s4wave_world.SRPCTypedObjectResourceServiceServer     = (*EngineResource)(nil)
)
