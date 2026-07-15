package resource_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_bucket_lookup "github.com/s4wave/spacewave/core/resource/bucket/lookup"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// EngineResource wraps an Engine for resource access.
type EngineResource struct {
	le                *logrus.Entry
	b                 bus.Bus
	mux               srpc.Invoker
	engine            world.Engine
	lookupOp          world.LookupOp
	engineInfo        *s4wave_world.EngineInfo
	worldStateOptions []WorldStateResourceOption
	typedResource     *TypedObjectResource
}

// NewEngineResource creates a new EngineResource.
func NewEngineResource(
	le *logrus.Entry,
	b bus.Bus,
	w world.Engine,
	lookupOp world.LookupOp,
	engineInfo *s4wave_world.EngineInfo,
	opts ...WorldStateResourceOption,
) *EngineResource {
	sessionPeerID, sessionPeerIDBound := worldStateResourceSessionPeerID(opts...)
	engineResource := &EngineResource{
		le:                le,
		b:                 b,
		engine:            w,
		lookupOp:          lookupOp,
		engineInfo:        engineInfo,
		worldStateOptions: opts,
	}
	engineResource.typedResource = newTypedObjectResourceWithSessionPeerID(
		le,
		b,
		world.NewEngineWorldState(w, true),
		w,
		sessionPeerID,
		sessionPeerIDBound,
	)
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

// GetWorldRootSnapshot returns the current committed World root.
func (r *EngineResource) GetWorldRootSnapshot(ctx context.Context, req *s4wave_world.GetWorldRootSnapshotRequest) (*s4wave_world.WorldRootSnapshot, error) {
	return r.loadWorldRootSnapshot(ctx)
}

// WatchWorldRootSnapshots streams committed World root snapshots.
func (r *EngineResource) WatchWorldRootSnapshots(
	req *s4wave_world.WatchWorldRootSnapshotsRequest,
	stream s4wave_world.SRPCEngineResourceService_WatchWorldRootSnapshotsStream,
) error {
	ctx := stream.Context()
	var sentSeqno uint64
	var sent bool
	for {
		snapshot, err := r.loadWorldRootSnapshot(ctx)
		if err != nil {
			return err
		}
		if !sent || snapshot.GetSeqno() != sentSeqno {
			if err := stream.Send(snapshot); err != nil {
				return err
			}
			sentSeqno = snapshot.GetSeqno()
			sent = true
		}
		_, err = r.engine.WaitSeqno(ctx, sentSeqno+1)
		if err != nil {
			return err
		}
	}
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

// Sync fences durable storage and advances the durable head via the engine.
func (r *EngineResource) Sync(ctx context.Context, req *s4wave_world.SyncRequest) (*s4wave_world.SyncResponse, error) {
	fenced, err := r.engine.Sync(ctx)
	if err != nil {
		return nil, err
	}
	if fenced {
		notifyDurableMutationToBrowser()
	}
	return &s4wave_world.SyncResponse{Fenced: fenced}, nil
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

	txResource := NewTxResource(r.le, r.b, wtx, r.lookupOp, r.engine, r.worldStateOptions...)
	id, err := resourceCtx.AddResource(txResource.GetMux(), func() {
		txResource.Release()
	})
	if err != nil {
		txResource.Release()
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

func (r *EngineResource) loadWorldRootSnapshot(ctx context.Context) (*s4wave_world.WorldRootSnapshot, error) {
	wtx, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer wtx.Discard()

	seqno, err := wtx.GetSeqno(ctx)
	if err != nil {
		return nil, err
	}
	var rootRef *bucket.ObjectRef
	var storageVolumeID string
	err = wtx.AccessWorldState(ctx, nil, func(c *bucket_lookup.Cursor) error {
		rootRef = c.GetRefWithOpArgs()
		storageVolumeID = c.GetOpArgs().GetVolumeId()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &s4wave_world.WorldRootSnapshot{
		RootRef:         rootRef,
		Seqno:           seqno,
		EngineInfo:      r.engineInfo,
		StorageVolumeId: storageVolumeID,
	}, nil
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

		// Create new tracked WorldState (empty - no tracking yet).
		// Client reads use the initial snapshot, while the watcher checks the
		// current engine state so committed updates can invalidate the snapshot.
		trackedWs := NewTrackedWorldState(wtx, world.NewEngineWorldState(r.engine, false), seqno, ctx)

		// Register as a resource
		trackedResource := NewEngineWorldStateResource(r.le, r.b, trackedWs, r.lookupOp, r.engine, r.worldStateOptions...)
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
	return r.typedResource.AccessTypedObject(ctx, req)
}

// _ is a type assertion
var (
	_ s4wave_world.SRPCEngineResourceServiceServer          = (*EngineResource)(nil)
	_ s4wave_world.SRPCWatchWorldStateResourceServiceServer = (*EngineResource)(nil)
	_ s4wave_world.SRPCTypedObjectResourceServiceServer     = (*EngineResource)(nil)
)
