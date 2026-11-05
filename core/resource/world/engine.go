package resource_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_bucket_lookup "github.com/s4wave/spacewave/core/resource/bucket/lookup"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// EngineResource wraps an Engine for resource access.
type EngineResource struct {
	// le logs Resource and controller lifecycle failures.
	le *logrus.Entry
	// b resolves World and typed handler directives.
	b bus.Bus
	// mux serves the mounted Resource RPCs.
	mux srpc.Invoker
	// engine retains the granted transactional World capability.
	engine world.Engine
	// lookupOp resolves World operations under the granting mount.
	lookupOp world.LookupOp
	// engineInfo describes the engine selected by the granting mount.
	engineInfo *s4wave_world.EngineInfo
	// worldStateOptions carries trusted access options into World descendants.
	worldStateOptions []WorldStateResourceOption
	// typedResource owns the typed handles created under this mount.
	typedResource *TypedObjectResource
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
	// Bind the granting mount identity before constructing any World descendants.
	if engineInfo != nil {
		opts = append(opts, WithEngineID(engineInfo.GetEngineId()))
	}

	// Capture trusted access options and initialize the resource wrapper.
	engineResource := &EngineResource{
		le:                le,
		b:                 b,
		engine:            w,
		lookupOp:          lookupOp,
		engineInfo:        engineInfo,
		worldStateOptions: opts,
	}

	// Attach typed-object access to the world engine.
	engineResource.typedResource = NewTypedObjectResource(le, b, world.NewEngineWorldState(w, true), w, opts...)

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

// GetEngine returns the capability already granted by this Engine Resource.
func (r *EngineResource) GetEngine() world.Engine {
	return r.engine
}

// Close releases typed object handles owned by this resource mount.
// It leaves the supplied engine and its storage running.
func (r *EngineResource) Close() {
	r.typedResource.Close()
}

// GetEngineInfo returns information about the world engine.
func (r *EngineResource) GetEngineInfo(ctx context.Context, req *s4wave_world.GetEngineInfoRequest) (*s4wave_world.GetEngineInfoResponse, error) {
	// Resolve accepted caller identity from the granting mount.
	sessionPeerID, _ := worldStateResourceSessionPeerID(r.worldStateOptions...)
	access := new(WorldStateResource)
	applyWorldStateResourceOptions(access, r.worldStateOptions...)
	var participant *sobject.SOParticipantConfig
	if access.participantLookup != nil && sessionPeerID != "" {
		var err error
		participant, err = access.participantLookup(ctx, sessionPeerID)
		if err != nil && !errors.Is(err, sobject.ErrNotParticipant) {
			return nil, err
		}
	}

	// Return mount metadata with its accepted caller identity.
	return &s4wave_world.GetEngineInfoResponse{
		EngineInfo: r.engineInfo, SessionPeerId: sessionPeerID.String(), CallerParticipant: participant,
	}, nil
}

// ExecuteWorldOp delegates transaction acceptance and stale-base retry to World.
func (r *EngineResource) ExecuteWorldOp(ctx context.Context, req *s4wave_world.ApplyWorldOpRequest) (*s4wave_world.ApplyWorldOpResponse, error) {
	state := NewWorldStateResource(r.le, r.b, world.NewEngineWorldState(r.engine, true), r.lookupOp, r.worldStateOptions...)
	return state.ApplyWorldOp(ctx, req)
}

// GetWorldRootSnapshot returns the current committed World root.
func (r *EngineResource) GetWorldRootSnapshot(ctx context.Context, req *s4wave_world.GetWorldRootSnapshotRequest) (*s4wave_world.WorldRootSnapshot, error) {
	return r.loadWorldRootSnapshot(ctx)
}

// SetRetainedRoot retains a World root in an engine that supports retention.
func (r *EngineResource) SetRetainedRoot(ctx context.Context, req *s4wave_world.SetRetainedRootRequest) (*s4wave_world.SetRetainedRootResponse, error) {
	err := world.AccessEngine(ctx, r.engine, func(ctx context.Context, eng world.Engine) error {
		retainer, ok := eng.(rootRetainingEngine)
		if !ok {
			return errors.New("world engine does not retain roots")
		}
		return retainer.SetRetainedRoot(ctx, req.GetName(), req.GetRootRef())
	})
	if err != nil {
		return nil, err
	}
	return &s4wave_world.SetRetainedRootResponse{}, nil
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

// SyncThrough fences durable storage through the requested seqno via the engine.
func (r *EngineResource) SyncThrough(ctx context.Context, req *s4wave_world.SyncThroughRequest) (*s4wave_world.SyncThroughResponse, error) {
	// Fence the engine, then notify browser observers of the new durable head.
	durable, err := r.engine.SyncThrough(ctx, req.GetSeqno())
	if err != nil {
		return nil, err
	}
	notifyDurableMutationToBrowser()
	return &s4wave_world.SyncThroughResponse{DurableSeqno: durable}, nil
}

// WaitSeqno waits for the seqno of the world state to be >= value.
func (r *EngineResource) WaitSeqno(ctx context.Context, req *s4wave_world.WaitSeqnoRequest) (*s4wave_world.WaitSeqnoResponse, error) {
	seqno, err := r.engine.WaitSeqno(ctx, req.GetSeqno())
	if err != nil {
		return nil, err
	}
	return &s4wave_world.WaitSeqnoResponse{Seqno: seqno}, nil
}

// WatchChanges streams each later World revision with the changes since the
// previous message.
func (r *EngineResource) WatchChanges(
	req *s4wave_world.WatchChangesRequest,
	stream s4wave_world.SRPCEngineResourceService_WatchChangesStream,
) error {
	return world.WatchChanges(stream.Context(), r.engine, req.GetAfterSeqno(), func(seqno uint64, changes *world.ChangeSet) error {
		return stream.Send(s4wave_world.NewWorldChange(seqno, changes))
	})
}

// WaitObjectRev waits for an object revision through the engine's key watch.
// A missing object is reported as a response error code so the client can
// restore ErrObjectNotFound.
func (r *EngineResource) WaitObjectRev(ctx context.Context, req *s4wave_world.WaitObjectRevRequest) (*s4wave_world.WaitObjectRevResponse, error) {
	rev, err := r.engine.WaitObjectRev(ctx, req.GetKey(), req.GetRev(), req.GetIgnoreNotFound())
	if errors.Is(err, world.ErrObjectNotFound) {
		return &s4wave_world.WaitObjectRevResponse{ErrorCode: s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_OBJECT_NOT_FOUND}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s4wave_world.WaitObjectRevResponse{Rev: rev}, nil
}

// NewTransaction creates a new transaction against the world state.
func (r *EngineResource) NewTransaction(ctx context.Context, req *s4wave_world.NewTransactionRequest) (*s4wave_world.NewTransactionResponse, error) {
	// Acquire the resource client and open a transaction.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Acquire the transaction and transfer its release to the Resource client.
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

	return &s4wave_world.NewTransactionResponse{
		ResourceId: id,
		ReadOnly:   wtx.GetReadOnly(),
	}, nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
func (r *EngineResource) BuildStorageCursor(ctx context.Context, req *s4wave_world.BuildStorageCursorRequest) (*s4wave_world.BuildStorageCursorResponse, error) {
	// Acquire the resource client and build a storage cursor.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Acquire the storage cursor and transfer its release to the Resource client.
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

	// Register the cursor while its AccessWorldState callback stays open.
	id, err := addAccessWorldStateResource(ctx, resourceCtx, r.le, r.b, func(ctx context.Context, cb func(*bucket_lookup.Cursor) error) error {
		return r.engine.AccessWorldState(ctx, req.GetRef(), cb)
	})
	if err != nil {
		return nil, err
	}

	return &s4wave_world.AccessWorldStateResponse{ResourceId: id}, nil
}

// StageWorldState opens a World stage owned by a stage resource.
func (r *EngineResource) StageWorldState(ctx context.Context, req *s4wave_world.StageWorldStateRequest) (*s4wave_world.StageWorldStateResponse, error) {
	return addWorldStageResource(ctx, r.le, r.b, r.engine.StageWorldState)
}

// loadWorldRootSnapshot reads the committed root through one engine read scope.
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
//
// Each response names a tracked WorldState snapshot resource. Change detection
// starts as the client reads through it. After a change, the watch sends the
// replacement snapshot before it releases the superseded one, so a client
// whose call fails because its snapshot was released has the replacement on
// this stream. When the watch ends, it releases its current snapshot. Child
// resources the client adopted keep their snapshot readable until released.
func (r *EngineResource) WatchWorldState(
	req *s4wave_world.WatchWorldStateRequest,
	stream s4wave_world.SRPCWatchWorldStateResourceService_WatchWorldStateStream,
) error {
	// Resolve the resource client and release the current snapshot at exit.
	ctx := stream.Context()
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return err
	}
	var currentID uint32
	defer func() {
		if currentID != 0 {
			_ = resourceCtx.ReleaseResource(currentID)
		}
	}()

	// Publish a fresh snapshot after each observed change.
	for {
		// Register a snapshot resource that records what the client reads.
		reads := world.NewReadSet()
		seqno, resourceID, err := r.addTrackedWorldState(ctx, resourceCtx, reads)
		if err != nil {
			return err
		}

		// Publish it, then release the snapshot it supersedes.
		err = stream.Send(&s4wave_world.WatchWorldStateResponse{
			ResourceId: resourceID,
		})
		if err != nil {
			_ = resourceCtx.ReleaseResource(resourceID)
			return err
		}
		if currentID != 0 {
			_ = resourceCtx.ReleaseResource(currentID)
		}
		currentID = resourceID

		// Wait for a change to something the client read.
		if _, err := reads.WaitChange(ctx, r.engine, seqno); err != nil {
			return err
		}
	}
}

// addTrackedWorldState registers a WorldState resource over a new read
// transaction that records its reads in reads, and returns the transaction's
// seqno. The transaction is discarded once the resource and all of its
// adopted descendants are released.
func (r *EngineResource) addTrackedWorldState(
	ctx context.Context,
	resourceCtx resource_server.ResourceClientContext,
	reads *world.ReadSet,
) (uint64, uint32, error) {
	// Begin the snapshot transaction and capture its sequence.
	wtx, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return 0, 0, err
	}
	seqno, err := wtx.GetSeqno(ctx)
	if err != nil {
		wtx.Discard()
		return 0, 0, err
	}

	// Register the tracked resource over the transaction lease.
	trackedResource := NewEngineWorldStateResource(r.le, r.b, reads.Track(wtx), r.lookupOp, r.engine, r.worldStateOptions...)
	txLease := newResourceLease(wtx.Discard)
	release := func() {
		trackedResource.Close()
		txLease.releaseRef()
	}
	resourceID, err := resourceCtx.AddResource(txLease.wrapInvoker(trackedResource.GetMux()), release)
	if err != nil {
		release()
		return 0, 0, err
	}
	return seqno, resourceID, nil
}

// AccessTypedObject looks up an object, determines its type, and returns a typed resource.
func (r *EngineResource) AccessTypedObject(ctx context.Context, req *s4wave_world.AccessTypedObjectRequest) (*s4wave_world.AccessTypedObjectResponse, error) {
	return r.typedResource.AccessTypedObject(ctx, req)
}

// WatchTypedObject forwards standing typed demand under the Engine mount's authority.
func (r *EngineResource) WatchTypedObject(req *s4wave_world.WatchTypedObjectRequest, stream s4wave_world.SRPCTypedObjectResourceService_WatchTypedObjectStream) error {
	return r.typedResource.WatchTypedObject(req, stream)
}

// rootRetainingEngine is a Space World engine that retains named World roots
// in its shared state. Only the SharedObject World engine implements it; the
// Space adapter replaces it with roots chosen on the Space.
type rootRetainingEngine interface {
	// SetRetainedRoot retains the World root ref under name, replacing the
	// root the name held. An empty ref releases the name.
	SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error
}

// _ is a type assertion
var (
	_ s4wave_world.SRPCEngineResourceServiceServer          = (*EngineResource)(nil)
	_ s4wave_world.SRPCWatchWorldStateResourceServiceServer = (*EngineResource)(nil)
	_ s4wave_world.SRPCTypedObjectResourceServiceServer     = (*EngineResource)(nil)
)
