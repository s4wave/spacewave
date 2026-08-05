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
	// Capture trusted access options and initialize the resource wrapper.
	sessionPeerID, sessionPeerIDBound := worldStateResourceSessionPeerID(opts...)
	engineResource := &EngineResource{
		le:                le,
		b:                 b,
		engine:            w,
		lookupOp:          lookupOp,
		engineInfo:        engineInfo,
		worldStateOptions: opts,
	}

	// Attach typed-object access to the world engine.
	engineResource.typedResource = newTypedObjectResourceWithSessionPeerID(
		le,
		b,
		world.NewEngineWorldState(w, true),
		w,
		sessionPeerID,
		sessionPeerIDBound,
	)

	// Register world, watch, and typed-object RPC services.
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
	// Load and emit each changed root snapshot.
	ctx := stream.Context()
	var sentSeqno uint64
	var sent bool
	for {
		// Read the current committed root.
		snapshot, err := r.loadWorldRootSnapshot(ctx)
		if err != nil {
			return err
		}

		// Send a changed snapshot to the client.
		if !sent || snapshot.GetSeqno() != sentSeqno {
			if err := stream.Send(snapshot); err != nil {
				return err
			}
			sentSeqno = snapshot.GetSeqno()
			sent = true
		}

		// Wait for the next committed sequence.
		_, err = r.engine.WaitSeqno(ctx, sentSeqno+1)
		if err != nil {
			return err
		}
	}
}

// GetSeqno returns the current seqno of the world state.
func (r *EngineResource) GetSeqno(ctx context.Context, req *s4wave_world.GetSeqnoRequest) (*s4wave_world.GetSeqnoResponse, error) {
	// Open a read transaction for the current sequence.
	wtx, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer wtx.Discard()

	// Read the transaction sequence.
	seqno, err := wtx.GetSeqno(ctx)
	if err != nil {
		return nil, err
	}

	return &s4wave_world.GetSeqnoResponse{Seqno: seqno}, nil
}

// Sync fences durable storage and advances the durable head via the engine.
func (r *EngineResource) Sync(ctx context.Context, req *s4wave_world.SyncRequest) (*s4wave_world.SyncResponse, error) {
	// Flush durable state and notify browser observers.
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
	// Acquire the resource client and open a transaction.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	wtx, err := r.engine.NewTransaction(ctx, req.GetWrite())
	if err != nil {
		return nil, err
	}

	// Register the transaction resource with its release hook.
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
	// Acquire the resource client and build a storage cursor.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	cursor, err := r.engine.BuildStorageCursor(ctx)
	if err != nil {
		return nil, err
	}

	// Register the cursor resource with its release hook.
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
	// Acquire the resource client and build a world-state cursor.
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

	// Register the world-state cursor resource.
	id, err := resourceCtx.AddResource(cursorResource.GetMux(), func() {})
	if err != nil {
		return nil, err
	}

	return &s4wave_world.AccessWorldStateResponse{ResourceId: id}, nil
}

func (r *EngineResource) loadWorldRootSnapshot(ctx context.Context) (*s4wave_world.WorldRootSnapshot, error) {
	// Read the root sequence and storage reference.
	wtx, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer wtx.Discard()

	// Capture the current world sequence.
	seqno, err := wtx.GetSeqno(ctx)
	if err != nil {
		return nil, err
	}
	var rootRef *bucket.ObjectRef
	var storageVolumeID string

	// Read the current root reference and storage volume.
	err = wtx.AccessWorldState(ctx, nil, func(c *bucket_lookup.Cursor) error {
		rootRef = c.GetRefWithOpArgs()
		storageVolumeID = c.GetOpArgs().GetVolumeId()
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Assemble the root snapshot response.
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
		// Begin a tracked snapshot transaction.
		wtx, err := r.engine.NewTransaction(ctx, false)
		if err != nil {
			return err
		}

		// Capture the current sequence.
		seqno, err := wtx.GetSeqno(ctx)
		if err != nil {
			wtx.Discard()
			return err
		}

		// Build a tracked WorldState for client reads.
		trackedWs := NewTrackedWorldState(wtx, world.NewEngineWorldState(r.engine, false), seqno, ctx)

		// Register the tracked resource.
		trackedResource := NewEngineWorldStateResource(r.le, r.b, trackedWs, r.lookupOp, r.engine, r.worldStateOptions...)
		resourceId, err := resourceCtx.AddResource(trackedResource.GetMux(), func() {
			trackedWs.Close()
		})
		if err != nil {
			trackedWs.Close()
			return err
		}

		// Publish its resource ID.
		err = stream.Send(&s4wave_world.WatchWorldStateResponse{
			ResourceId: resourceId,
		})
		if err != nil {
			resourceCtx.ReleaseResource(resourceId)
			return err
		}

		// Wait for an observed world change.
		err = trackedWs.WaitForChanges(ctx)

		// Release the previous tracked resource.
		_ = resourceCtx.ReleaseResource(resourceId)

		// Return terminal watch errors.
		if err != nil {
			return err
		}

		// Continue with a fresh tracked WorldState after a change.
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
