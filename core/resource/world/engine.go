package resource_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_bucket_lookup "github.com/s4wave/spacewave/core/resource/bucket/lookup"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// errBatchRefused ends a batch attempt so its transaction is discarded.
var errBatchRefused = errors.New("world batch refused")

// batchOp is an op entry of a batch, resolved, decoded and prepared before
// the batch's transaction opens.
type batchOp struct {
	// op is the decoded operation.
	op world.Operation
	// sender is the peer the op is applied as.
	sender peer.ID
}

// EngineResource wraps an Engine for resource access.
type EngineResource struct {
	// le logs Resource and controller lifecycle failures.
	le *logrus.Entry
	// b resolves World and typed handler directives.
	b bus.Bus
	// mux serves the mounted Resource RPCs.
	mux srpc.Invoker
	// engine retains the granted transactional World capability, guarded when
	// the mount has a write guard.
	engine world.Engine
	// retainer is the unguarded engine, used only to retain roots: retaining a
	// root writes no World object, so the write guard does not cover it.
	retainer world.Engine
	// lookupOp resolves World operations under the granting mount.
	lookupOp world.LookupOp
	// engineInfo describes the engine selected by the granting mount.
	engineInfo *s4wave_world.EngineInfo
	// worldStateOptions carries trusted access options into World descendants.
	worldStateOptions []WorldStateResourceOption
	// mountEngine registers descendant Engines under the same mount policy.
	mountEngine func(resource_server.ResourceClientContext, world.Engine, ...WorldStateResourceOption) (uint32, error)
	// typedResource owns the typed handles created under this mount.
	typedResource *TypedObjectResource
}

// newEngineResource binds Engine RPCs to descendant Engine registration.
func newEngineResource(
	le *logrus.Entry,
	b bus.Bus,
	w world.Engine,
	lookupOp world.LookupOp,
	engineInfo *s4wave_world.EngineInfo,
	mountEngine func(resource_server.ResourceClientContext, world.Engine, ...WorldStateResourceOption) (uint32, error),
	opts ...WorldStateResourceOption,
) *EngineResource {
	// Guard the engine itself, so no handle or factory under this mount can
	// open a transaction past the mount's write guard.
	access := newMountOptions(opts...)
	retainer, w := w, world.GuardEngine(w, access.writeGuard)

	// Capture trusted access options and initialize the resource wrapper.
	engineResource := &EngineResource{
		le:                le,
		b:                 b,
		engine:            w,
		retainer:          retainer,
		lookupOp:          lookupOp,
		engineInfo:        engineInfo,
		worldStateOptions: opts,
		mountEngine:       mountEngine,
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
	access := newMountOptions(r.worldStateOptions...)
	sessionPeerID := access.sessionPeerID
	participant, err := access.lookupParticipant(ctx)
	if err != nil {
		return nil, err
	}

	// Report the signer of the operations this Resource applies, which
	// differs from the session peer when the engine signs as its Space device.
	device, person, err := r.engine.OperationAuthor(ctx)
	if err != nil {
		return nil, err
	}

	// Return mount metadata with its accepted caller identity.
	return &s4wave_world.GetEngineInfoResponse{
		EngineInfo:        r.engineInfo,
		SessionPeerId:     sessionPeerID.String(),
		CallerParticipant: participant,
		OperationPeerId:   device.String(),
		OperationPerson:   person,
	}, nil
}

// ExecuteWorldOp delegates transaction acceptance and stale-base retry to World.
func (r *EngineResource) ExecuteWorldOp(ctx context.Context, req *s4wave_world.ApplyWorldOpRequest) (*s4wave_world.ApplyWorldOpResponse, error) {
	state := newWorldStateResource(
		r.le, r.b, world.NewEngineWorldState(r.engine, true), r.lookupOp, srpc.NewMux(), nil, false, r.mountEngine, newMountOptions(r.worldStateOptions...),
	)
	return state.ApplyWorldOp(ctx, req)
}

// GetWorldRootSnapshot returns the current committed World root.
func (r *EngineResource) GetWorldRootSnapshot(ctx context.Context, req *s4wave_world.GetWorldRootSnapshotRequest) (*s4wave_world.WorldRootSnapshot, error) {
	return r.loadWorldRootSnapshot(ctx)
}

// SetRetainedRoot retains a World root in an engine that supports retention.
func (r *EngineResource) SetRetainedRoot(ctx context.Context, req *s4wave_world.SetRetainedRootRequest) (*s4wave_world.SetRetainedRootResponse, error) {
	err := world.AccessEngine(ctx, r.retainer, func(ctx context.Context, eng world.Engine) error {
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

// WatchTypeCounts streams every type count, then the counts each commit moves.
func (r *EngineResource) WatchTypeCounts(req *s4wave_world.WatchTypeCountsRequest, strm s4wave_world.SRPCEngineResourceService_WatchTypeCountsStream) error {
	return watchTypeCountEntries(strm.Context(), r.b, world.NewEngineWorldState(r.engine, false), strm.Send)
}

// SearchObjectsWithType streams the objects of a type whose key or title holds
// the query, in pages, from one read transaction, so every page reads the same
// committed World.
func (r *EngineResource) SearchObjectsWithType(
	req *s4wave_world.SearchObjectsWithTypeRequest, strm s4wave_world.SRPCEngineResourceService_SearchObjectsWithTypeStream) error {
	// Hold one read snapshot for the whole stream.
	ctx := strm.Context()
	tx, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Search the snapshot.
	return searchObjectsWithType(ctx, r.b, tx, req, strm.Send)
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
	// A write the World refuses before its turn opens returns as its code to
	// a client that decodes it, and as the call's error otherwise.
	wtx, err := r.engine.NewTransaction(ctx, req.GetWrite())
	code, err := writeErrorCode(req.GetKnownErrorCode(), err)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return &s4wave_world.NewTransactionResponse{ErrorCode: code}, nil
	}

	// Register the transaction resource with its release hook.
	txResource := newTxResource(r.le, r.b, wtx, r.lookupOp, r.engine, r.mountEngine, r.worldStateOptions...)
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
// Each response contains the ID of a tracked WorldState snapshot resource.
// Change detection starts as the client reads through the snapshot. After a
// change, the watch sends the replacement snapshot before releasing the
// superseded one, so a client whose call fails because its snapshot was
// released has the replacement on this stream. When the watch ends, it releases
// its current snapshot. Child resources the client adopted keep their snapshot
// readable until released.
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
	trackedResource := newWorldStateResource(
		r.le, r.b, reads.Track(wtx), r.lookupOp, srpc.NewMux(), r.engine, true, r.mountEngine, newMountOptions(r.worldStateOptions...),
	)
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
// in its shared state. The Space adapter's engine implements it, holding each
// as a root chosen on the Space.
type rootRetainingEngine interface {
	// SetRetainedRoot retains the World root ref under name, replacing the
	// root the name held. An empty ref releases the name.
	SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error
}

// ExecuteWorldBatch applies the mutations in order in one write transaction and
// commits it. The first refused mutation discards the transaction and reports
// its index. A stale base replays the whole list in a fresh transaction.
//
// Every op entry is resolved and prepared before the transaction takes the
// engine's writer, so a cold plugin load costs this caller time while other
// writers proceed, and ends with the caller's context error. An op the mount
// cannot resolve, or whose plugin load fails or passes its deadline, refuses
// the batch before any transaction opens, whatever entries precede it. Because
// a refused batch discards its transaction, a plugin that times out inside it
// is reported as WORLD_ERROR_CODE_PLUGIN_OP_TIMEOUT: nothing was written. So is
// a batch whose replays spent the write budget. That refusal identifies no
// entry, and its failed index is zero.
func (r *EngineResource) ExecuteWorldBatch(ctx context.Context, req *s4wave_world.ExecuteWorldBatchRequest) (*s4wave_world.ExecuteWorldBatchResponse, error) {
	// Resolve and prepare every op before the transaction takes the writer.
	mutations := req.GetMutations()
	access := newMountOptions(r.worldStateOptions...)
	ops, refused, release, err := prepareBatchOps(ctx, access, r.lookupOp, mutations)
	defer release()
	if err != nil || refused != nil {
		return refused, err
	}

	// Apply the mutations in order, stopping at the first refusal.
	var resp *s4wave_world.ExecuteWorldBatchResponse
	err = world.ExecTransaction(ctx, r.engine, true, func(ctx context.Context, wtx world.WorldState) error {
		// Record the failed index on a refusal and discard the
		// transaction.
		ws := world.GuardWorldState(wtx, access.writeGuard)
		results := make([]*s4wave_world.WorldMutationResult, len(mutations))
		for i, m := range mutations {
			rev, refusal, err := applyWorldMutation(ctx, ws, m, ops[i])
			if err != nil {
				refusal, err = batchOpRefusal(err)
			}
			if err != nil {
				return errors.Wrapf(err, "mutations[%d]", i)
			}
			if refusal != nil {
				refusal.FailedIndex = uint32(i)
				resp = refusal
				return errBatchRefused
			}
			results[i] = &s4wave_world.WorldMutationResult{Rev: rev}
		}

		// Report the sequence number the commit publishes.
		seqno, err := wtx.GetSeqno(ctx)
		if err != nil {
			return err
		}
		resp = &s4wave_world.ExecuteWorldBatchResponse{Seqno: seqno, Results: results}
		return nil
	})
	if err != nil && !errors.Is(err, errBatchRefused) {
		resp, err = batchOpRefusal(err)
		if err != nil {
			return nil, err
		}
	}

	// Return an unknown outcome to a client that cannot decode its code as
	// the bare durability unknown error.
	unknown := s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_DURABILITY_UNKNOWN
	if resp.GetErrorCode() == unknown && !s4wave_world.Decodes(req.GetKnownErrorCode(), unknown) {
		return nil, world.ErrDurabilityUnknown
	}
	return resp, nil
}

// prepareBatchOps resolves, decodes and prepares the World and object op
// entries of mutations in order, outside any transaction, with access holding
// the mount options ExecuteWorldOp resolves ops with. ops is indexed like
// mutations and holds nil for entries that are not ops. The first op the
// mount cannot resolve, or whose preparation is refused, such as by a
// rejection or an unavailable plugin, stops the walk and is returned as the
// refusal at its index, and the batch then opens no transaction. release
// frees every prepared op and must be called once the transaction has ended.
func prepareBatchOps(
	ctx context.Context,
	access mountOptions,
	lookupOp world.LookupOp,
	mutations []*s4wave_world.WorldMutation,
) (ops []*batchOp, refused *s4wave_world.ExecuteWorldBatchResponse, release func(), err error) {
	// Prepare each op in order, collecting the releases.
	var releases []func()
	release = func() {
		for _, rel := range releases {
			rel()
		}
	}
	ops = make([]*batchOp, len(mutations))
	for i, m := range mutations {
		opReq := m.GetOp()
		if objOp := m.GetApplyObjectOp(); objOp != nil {
			opReq = &s4wave_world.ApplyWorldOpRequest{
				OpTypeId: objOp.GetOpTypeId(), OpData: objOp.GetOpData(), OpSender: objOp.GetOpSender(),
			}
		}
		if opReq == nil {
			continue
		}
		op, sender, opRefused, err := access.resolveWorldOp(ctx, lookupOp, opReq)
		if err != nil {
			return nil, nil, release, errors.Wrapf(err, "mutations[%d]", i)
		}
		if opRefused != nil {
			return ops, &s4wave_world.ExecuteWorldBatchResponse{
				FailedIndex: uint32(i),
				ErrorCode:   opRefused.GetErrorCode(),
			}, release, nil
		}
		rel, err := world.PrepareOp(ctx, op)
		if err != nil {
			prepRefused, prepErr := batchOpRefusal(err)
			if prepErr != nil {
				return nil, nil, release, errors.Wrapf(prepErr, "mutations[%d]", i)
			}
			prepRefused.FailedIndex = uint32(i)
			return ops, prepRefused, release, nil
		}
		releases = append(releases, rel)
		ops[i] = &batchOp{op: op, sender: sender}
	}
	return ops, nil, release, nil
}

// applyWorldMutation applies one mutation to ws, with op holding the prepared
// operation of an op entry. It returns the object revision an object mutation
// leaves, and the refusal for an entry whose object is absent. The caller
// classifies a returned error with batchOpRefusal.
func applyWorldMutation(
	ctx context.Context,
	ws world.WorldState,
	m *s4wave_world.WorldMutation,
	op *batchOp,
) (uint64, *s4wave_world.ExecuteWorldBatchResponse, error) {
	switch m := m.GetMutation().(type) {
	case *s4wave_world.WorldMutation_Op:
		_, _, err := ws.ApplyWorldOp(ctx, op.op, op.sender)
		return 0, nil, err
	case *s4wave_world.WorldMutation_CreateObject:
		obj, err := ws.CreateObject(ctx, m.CreateObject.GetObjectKey(), m.CreateObject.GetRootRef())
		world.ReleaseObjectState(obj)
		return 0, nil, err
	case *s4wave_world.WorldMutation_RenameObject:
		req := m.RenameObject
		obj, err := ws.RenameObject(ctx, req.GetOldObjectKey(), req.GetNewObjectKey(), req.GetDescendants())
		world.ReleaseObjectState(obj)
		if errors.Is(err, world.ErrObjectNotFound) {
			return 0, objectNotFound(), nil
		}
		return 0, nil, err
	case *s4wave_world.WorldMutation_SetGraphQuad:
		q, err := batchGraphQuad(m.SetGraphQuad.GetQuad())
		if err != nil {
			return 0, nil, err
		}
		return 0, nil, ws.SetGraphQuad(ctx, q)
	case *s4wave_world.WorldMutation_DeleteGraphQuad:
		q, err := batchGraphQuad(m.DeleteGraphQuad.GetQuad())
		if err != nil {
			return 0, nil, err
		}
		return 0, nil, ws.DeleteGraphQuad(ctx, q)
	case *s4wave_world.WorldMutation_DeleteGraphObject:
		return 0, nil, ws.DeleteGraphObject(ctx, m.DeleteGraphObject.GetObjectKey())
	case *s4wave_world.WorldMutation_SetObjectType:
		return 0, nil, world_types.SetObjectType(ctx, ws, m.SetObjectType.GetObjectKey(), m.SetObjectType.GetTypeId())
	case *s4wave_world.WorldMutation_DeleteObject:
		deleted, err := ws.DeleteObject(ctx, m.DeleteObject.GetObjectKey())
		if err != nil || deleted {
			return 0, nil, err
		}
		return 0, objectNotFound(), nil
	case *s4wave_world.WorldMutation_SetRootRef:
		return applyObjectMutation(ctx, ws, m.SetRootRef.GetObjectKey(), func(obj world.ObjectState) (uint64, error) {
			return obj.SetRootRef(ctx, m.SetRootRef.GetRootRef())
		})
	case *s4wave_world.WorldMutation_ApplyObjectOp:
		return applyObjectMutation(ctx, ws, m.ApplyObjectOp.GetObjectKey(), func(obj world.ObjectState) (uint64, error) {
			rev, _, err := obj.ApplyObjectOp(ctx, op.op, op.sender)
			return rev, err
		})
	case *s4wave_world.WorldMutation_IncrementRev:
		return applyObjectMutation(ctx, ws, m.IncrementRev.GetObjectKey(), func(obj world.ObjectState) (uint64, error) {
			return obj.IncrementRev(ctx)
		})
	default:
		return 0, nil, errors.New("empty mutation")
	}
}

// applyObjectMutation calls apply with the object at key and returns the
// revision it leaves. An absent object refuses the batch.
func applyObjectMutation(
	ctx context.Context,
	ws world.WorldState,
	key string,
	apply func(obj world.ObjectState) (uint64, error),
) (uint64, *s4wave_world.ExecuteWorldBatchResponse, error) {
	// Acquire the object, refusing the batch when it is absent.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return 0, nil, err
	}
	if !found {
		return 0, objectNotFound(), nil
	}
	rev, err := apply(obj)
	return rev, nil, err
}

// batchOpRefusal returns the refusal an entry's error carries, such as a
// rejection by the op or the mount's write guard, or the error when it carries
// none. The batch discards its transaction on any refusal, so a plugin timeout
// is a refusal: nothing was kept.
func batchOpRefusal(err error) (*s4wave_world.ExecuteWorldBatchResponse, error) {
	// Classify the error, passing on one that carries no refusal.
	if err == nil {
		return nil, nil
	}
	code, rejection, ok := opErrorRefusal(err, true)
	if !ok {
		return nil, err
	}
	refusal := &s4wave_world.ExecuteWorldBatchResponse{ErrorCode: code}
	if rejection != nil {
		refusal.RejectionCode, refusal.RejectionMessage = rejection.Code, rejection.Message
	}
	return refusal, nil
}

// batchGraphQuad converts the quad of a graph quad entry.
func batchGraphQuad(q *quad.Quad) (world.GraphQuad, error) {
	if q == nil {
		return nil, errors.New("no graph quad")
	}
	return world.NewGraphQuad(q.GetSubject(), q.GetPredicate(), q.GetObj(), q.GetLabel()), nil
}

// objectNotFound is the refusal of an entry whose object is absent.
func objectNotFound() *s4wave_world.ExecuteWorldBatchResponse {
	return &s4wave_world.ExecuteWorldBatchResponse{
		ErrorCode: s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_OBJECT_NOT_FOUND,
	}
}

// _ is a type assertion
var (
	_ s4wave_world.SRPCEngineResourceServiceServer          = (*EngineResource)(nil)
	_ s4wave_world.SRPCWatchWorldStateResourceServiceServer = (*EngineResource)(nil)
	_ s4wave_world.SRPCTypedObjectResourceServiceServer     = (*EngineResource)(nil)
)
