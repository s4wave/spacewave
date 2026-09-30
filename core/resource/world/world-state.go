package resource_world

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_bucket_lookup "github.com/s4wave/spacewave/core/resource/bucket/lookup"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/quad"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// WorldStateResource wraps a WorldState for resource access.
type WorldStateResource struct {
	// le logs Resource and controller lifecycle failures.
	le *logrus.Entry
	// b resolves World and typed handler directives.
	b bus.Bus
	// mux serves the mounted Resource RPCs.
	mux srpc.Invoker
	// ws holds the granted World snapshot and write authority.
	ws world.WorldState
	// typedResource owns typed watches attached to this snapshot mount.
	typedResource *TypedObjectResource
	// lookupOp resolves World operations under the granting mount.
	lookupOp world.LookupOp
	// storage accesses the outer World or Engine to open nested World snapshots.
	storage world.WorldStorage
	// outerEngine is the authorizing engine retained only by nested World resources.
	outerEngine world.Engine

	// engineID is the registry scope granted by the mounting component.
	engineID string
	// engineIDBound prevents incoming caller context from changing registry scope.
	engineIDBound bool
	// options carries trusted mount options into nested and outer World resources.
	options []WorldStateResourceOption

	// sessionPeerID is the authenticated peer selected by the granting mount.
	sessionPeerID peer.ID
	// sessionPeerIDBound prevents request context from replacing the mount peer.
	sessionPeerIDBound bool
	// operationObserver receives operation accounting for this mount.
	operationObserver WorldStateOperationObserver
}

// NewWorldStateResource creates a new WorldStateResource.
//
// lookupOp may be nil.
func NewWorldStateResource(
	le *logrus.Entry,
	b bus.Bus,
	ws world.WorldState,
	lookupOp world.LookupOp,
	opts ...WorldStateResourceOption,
) *WorldStateResource {
	return newWorldStateResource(le, b, ws, lookupOp, nil, opts...)
}

// NewEngineWorldStateResource creates a WorldStateResource with typed object access.
func NewEngineWorldStateResource(
	le *logrus.Entry,
	b bus.Bus,
	ws world.WorldState,
	lookupOp world.LookupOp,
	engine world.Engine,
	opts ...WorldStateResourceOption,
) *WorldStateResource {
	return newWorldStateResource(le, b, ws, lookupOp, engine, opts...)
}

// newWorldStateResource binds World access and typed descendants to trusted mount options.
func newWorldStateResource(
	le *logrus.Entry,
	b bus.Bus,
	ws world.WorldState,
	lookupOp world.LookupOp,
	engine world.Engine,
	opts ...WorldStateResourceOption,
) *WorldStateResource {
	// Select the granting storage and capture its trusted Resource options.
	storage := world.WorldStorage(ws)
	if engine != nil {
		storage = engine
	}
	wsResource := &WorldStateResource{le: le, b: b, ws: ws, lookupOp: lookupOp, storage: storage, options: opts}
	applyWorldStateResourceOptions(wsResource, opts...)

	// Register World and optional typed access on the same Resource mux.
	register := []func(srpc.Mux) error{
		func(mux srpc.Mux) error {
			return s4wave_world.SRPCRegisterWorldStateResourceService(mux, wsResource)
		},
	}
	if engine != nil {
		typedResource := NewTypedObjectResource(le, b, ws, engine, opts...)
		wsResource.typedResource = typedResource
		register = append(register, func(mux srpc.Mux) error {
			return s4wave_world.SRPCRegisterTypedObjectResourceService(mux, typedResource)
		})
	}
	mux := resource_server.NewResourceMux(register...)
	wsResource.mux = mux
	return wsResource
}

// Close withdraws typed demand owned by this World snapshot mount.
func (r *WorldStateResource) Close() {
	if r.typedResource != nil {
		r.typedResource.Close()
	}
}

// GetMux returns the rpc mux.
func (r *WorldStateResource) GetMux() srpc.Invoker {
	return r.mux
}

// GetReadOnly returns if the world state is read-only.
func (r *WorldStateResource) GetReadOnly(ctx context.Context, req *s4wave_world.GetReadOnlyRequest) (*s4wave_world.GetReadOnlyResponse, error) {
	return &s4wave_world.GetReadOnlyResponse{ReadOnly: r.ws.GetReadOnly()}, nil
}

// Sync fences the block writes made through this world state durable.
func (r *WorldStateResource) Sync(ctx context.Context, req *s4wave_world.SyncRequest) (*s4wave_world.SyncResponse, error) {
	fenced, err := r.ws.Sync(ctx)
	if err != nil {
		return nil, err
	}
	if fenced {
		notifyDurableMutationToBrowser()
	}
	return &s4wave_world.SyncResponse{Fenced: fenced}, nil
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
	// Require the Resource client that will own the storage cursor.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Acquire a storage cursor from the mounted World.
	cursor, err := r.ws.BuildStorageCursor(ctx)
	if err != nil {
		return nil, err
	}

	// Transfer cursor release to the Resource client.
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
	// Require the Resource client that will own the World cursor.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Register the cursor while its AccessWorldState callback stays open.
	id, err := addAccessWorldStateResource(ctx, resourceCtx, r.le, r.b, func(ctx context.Context, cb func(*bucket_lookup.Cursor) error) error {
		return r.ws.AccessWorldState(ctx, req.GetRef(), cb)
	})
	if err != nil {
		return nil, err
	}

	return &s4wave_world.AccessWorldStateResponse{ResourceId: id}, nil
}

// OpenNestedWorld opens a typed object's immutable nested World snapshot.
// The returned resource owns its snapshot pin independently of the outer state.
func (r *WorldStateResource) OpenNestedWorld(ctx context.Context, req *s4wave_world.OpenNestedWorldRequest) (*s4wave_world.OpenNestedWorldResponse, error) {
	// Require the Resource client that will own the nested snapshot.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Validate the typed object and its nested World root.
	key := req.GetObjectKey()
	if key == "" {
		return nil, world.ErrEmptyObjectKey
	}
	typeID, err := world_types.GetObjectType(ctx, r.ws, key)
	if err != nil {
		return nil, err
	}
	if typeID == "" {
		return nil, errors.Errorf("object %s is not typed", key)
	}
	outer, err := world.LookupObjectBody[*world_block.NestedWorld](ctx, r.ws, key, world_block.NewNestedWorldBlock)
	if err != nil {
		return nil, err
	}
	if outer.GetWorldRef().GetRootRef() == nil {
		return nil, errors.Errorf("object %s has no nested World root", key)
	}

	// Open the immutable nested World through the granting storage scope.
	var nested *world_block.WorldState
	err = r.storage.AccessWorldState(ctx, outer.GetWorldRef(), func(cursor *bucket_lookup.Cursor) error {
		var buildErr error
		nested, buildErr = world_block.BuildWorldStateFromCursor(ctx, r.le, false, cursor, r.storage, r.lookupOp, false)
		return buildErr
	})
	if err != nil {
		return nil, err
	}

	// Carry the granting World options and storage into the nested Resource.
	resource := NewWorldStateResource(r.le, r.b, nested, r.lookupOp, r.options...)
	resource.storage = r.storage
	if engine, ok := r.storage.(world.Engine); ok {
		resource.outerEngine = engine
	}
	id, err := resourceCtx.AddResource(resource.GetMux(), nested.Discard)
	if err != nil {
		nested.Discard()
		return nil, err
	}
	return &s4wave_world.OpenNestedWorldResponse{ResourceId: id}, nil
}

// OpenOuterWorld grants the Space Engine that authorized this nested World.
// The Engine reads current Space state and writes under the same session
// authority, including World ops. It outlives this nested state.
func (r *WorldStateResource) OpenOuterWorld(ctx context.Context, _ *s4wave_world.OpenOuterWorldRequest) (*s4wave_world.OpenOuterWorldResponse, error) {
	// Require the outer World capability and its Resource client.
	if r.outerEngine == nil {
		return nil, errors.New("outer World is unavailable on this resource")
	}
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Transfer the outer Engine Resource to the client with its trusted mount options.
	resource := NewEngineResource(r.le, r.b, r.outerEngine, r.lookupOp, nil, r.options...)
	id, err := resourceCtx.AddResource(resource.GetMux(), resource.Close)
	if err != nil {
		resource.Close()
		return nil, err
	}
	return &s4wave_world.OpenOuterWorldResponse{ResourceId: id}, nil
}

// CreateObject creates an object with a key and initial root ref.
func (r *WorldStateResource) CreateObject(ctx context.Context, req *s4wave_world.CreateObjectRequest) (*s4wave_world.CreateObjectResponse, error) {
	// Require the Resource client that will own the created object.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Create the World object and retain its returned state.
	obj, err := r.ws.CreateObject(ctx, req.GetObjectKey(), req.GetRootRef())
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}

	// Transfer the created object state to the Resource client.
	key := obj.GetKey()
	objResource := r.newObjectStateResource(obj)
	id, err := resourceCtx.AddResource(objResource.GetMux(), func() { world.ReleaseObjectState(obj) })
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}

	return &s4wave_world.CreateObjectResponse{ResourceId: id, ObjectKey: key}, nil
}

// GetObject looks up an object by key.
func (r *WorldStateResource) GetObject(ctx context.Context, req *s4wave_world.GetObjectRequest) (*s4wave_world.GetObjectResponse, error) {
	// Require the Resource client that will own the requested object.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Read the object state from the mounted World.
	obj, found, err := r.ws.GetObject(ctx, req.GetObjectKey())
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}

	// Report an absent object before constructing its Resource.
	if !found {
		return &s4wave_world.GetObjectResponse{Found: false}, nil
	}

	// Transfer the object state to the Resource client.
	key := obj.GetKey()
	objResource := r.newObjectStateResource(obj)
	id, err := resourceCtx.AddResource(objResource.GetMux(), func() { world.ReleaseObjectState(obj) })
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}

	return &s4wave_world.GetObjectResponse{Found: true, ResourceId: id, ObjectKey: key}, nil
}

// IterateObjects returns an iterator with the given object key prefix.
func (r *WorldStateResource) IterateObjects(ctx context.Context, req *s4wave_world.IterateObjectsRequest) (*s4wave_world.IterateObjectsResponse, error) {
	// Require the Resource client that will own the object iterator.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// The iterator outlives this unary RPC. Resource release closes it, while
	// each iterator method supplies its own request context.
	iter := r.ws.IterateObjects(context.WithoutCancel(ctx), req.GetPrefix(), req.GetReversed())
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
	// Require the Resource client that will own the renamed object.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Rename the World object and retain its returned state.
	obj, err := r.ws.RenameObject(ctx, req.GetOldObjectKey(), req.GetNewObjectKey(), req.GetDescendants())
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}

	// Transfer the renamed object state to the Resource client.
	key := obj.GetKey()
	objResource := r.newObjectStateResource(obj)
	id, err := resourceCtx.AddResource(objResource.GetMux(), func() { world.ReleaseObjectState(obj) })
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}

	return &s4wave_world.RenameObjectResponse{ResourceId: id, ObjectKey: key}, nil
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
	// Apply the requested quad to the mounted World graph.
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
	// Remove the requested quad from the mounted World graph.
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
	// Resolve the requested graph filter against the mounted World.
	f := req.GetFilter()
	filter := world.NewGraphQuad(f.GetSubject(), f.GetPredicate(), f.GetObj(), f.GetLabel())
	quads, err := r.ws.LookupGraphQuads(ctx, filter, req.GetLimit())
	if err != nil {
		return nil, err
	}

	return &s4wave_world.LookupGraphQuadsResponse{Quads: graphQuadsToProto(quads)}, nil
}

// LookupGraphQuadsBatch searches for graph quads using bounded indexed filters.
func (r *WorldStateResource) LookupGraphQuadsBatch(ctx context.Context, req *s4wave_world.LookupGraphQuadsBatchRequest) (*s4wave_world.LookupGraphQuadsBatchResponse, error) {
	// Account for indexed graph reads through the mounted World.
	started := time.Now()
	record := WorldStateOperationRecord{
		Name:        "LookupGraphQuadsBatch",
		FilterCount: len(req.GetFilters()),
		Limit:       int(req.GetLimitPerFilter()),
	}
	var retErr error
	ctx, readCounter := block.WithReadCounter(ctx)
	defer func() {
		recordBlockReadSnapshot(&record, readCounter)
		r.observeOperation(record, started, retErr)
	}()

	// Reject unbounded remote graph filters.
	if req.GetLimitPerFilter() == 0 {
		retErr = errors.New("limit_per_filter must be non-zero")
		return nil, retErr
	}

	// Validate each filter and execute the bounded graph batch.
	filters := make([]world.GraphQuad, len(req.GetFilters()))
	for i, f := range req.GetFilters() {
		if err := validateBatchGraphFilter(f); err != nil {
			retErr = err
			return nil, retErr
		}
		filters[i] = world.NewGraphQuad(f.GetSubject(), f.GetPredicate(), f.GetObj(), f.GetLabel())
	}
	quads, err := r.ws.LookupGraphQuadsBatch(ctx, filters, req.GetLimitPerFilter())
	if err != nil {
		retErr = err
		return nil, retErr
	}

	// Encode graph results and record the returned quad counts.
	results := make([]*s4wave_world.LookupGraphQuadsBatchResult, len(quads))
	for i, resultQuads := range quads {
		record.ResultQuadCount += len(resultQuads)
		results[i] = &s4wave_world.LookupGraphQuadsBatchResult{
			Quads: graphQuadsToProto(resultQuads),
		}
	}
	record.ResultSetCount = len(results)

	return &s4wave_world.LookupGraphQuadsBatchResponse{Results: results}, nil
}

// ListGraphEdgeBuckets lists grouped inbound/outbound graph edge buckets.
func (r *WorldStateResource) ListGraphEdgeBuckets(ctx context.Context, req *s4wave_world.ListGraphEdgeBucketsRequest) (*s4wave_world.ListGraphEdgeBucketsResponse, error) {
	// Account for grouped graph reads through the mounted World.
	started := time.Now()
	record := WorldStateOperationRecord{
		Name:          "ListGraphEdgeBuckets",
		StartKeyCount: len(req.GetOriginObjectKeys()),
		Limit:         int(req.GetLimitPerOrigin()),
	}
	var retErr error
	ctx, readCounter := block.WithReadCounter(ctx)
	defer func() {
		recordBlockReadSnapshot(&record, readCounter)
		r.observeOperation(record, started, retErr)
	}()

	// Validate the requested graph edge direction.
	direction, err := graphEdgeBucketDirectionFromProto(req.GetDirection())
	if err != nil {
		retErr = err
		return nil, retErr
	}

	// Read the indexed edge buckets for the requested origins.
	buckets, err := world.ListGraphEdgeBuckets(ctx, r.ws, &world.GraphEdgeBucketQuery{
		OriginObjectKeys: req.GetOriginObjectKeys(),
		Predicate:        req.GetPredicate(),
		LimitPerOrigin:   req.GetLimitPerOrigin(),
		Direction:        direction,
	})
	if err != nil {
		retErr = err
		return nil, retErr
	}

	// Encode graph buckets and record the returned edge counts.
	out := make([]*s4wave_world.GraphEdgeBucket, len(buckets))
	for i, bucket := range buckets {
		record.ResultQuadCount += len(bucket.Outgoing) + len(bucket.Incoming)
		out[i] = &s4wave_world.GraphEdgeBucket{
			OriginObjectKey:   bucket.OriginObjectKey,
			Outgoing:          graphQuadsToProto(bucket.Outgoing),
			Incoming:          graphQuadsToProto(bucket.Incoming),
			OutgoingTruncated: bucket.OutgoingTruncated,
			IncomingTruncated: bucket.IncomingTruncated,
		}
	}
	record.ResultSetCount = len(out)

	return &s4wave_world.ListGraphEdgeBucketsResponse{Buckets: out}, nil
}

// ListObjectsWithType lists object keys with the given type identifier.
func (r *WorldStateResource) ListObjectsWithType(ctx context.Context, req *s4wave_world.ListObjectsWithTypeRequest) (*s4wave_world.ListObjectsWithTypeResponse, error) {
	objKeys, err := world_types.ListObjectsWithType(ctx, r.ws, req.GetTypeId())
	if err != nil {
		return nil, err
	}
	return &s4wave_world.ListObjectsWithTypeResponse{ObjectKeys: objKeys}, nil
}

// GetObjectRootRefsBatch returns root references for object keys.
func (r *WorldStateResource) GetObjectRootRefsBatch(ctx context.Context, req *s4wave_world.GetObjectRootRefsBatchRequest) (*s4wave_world.GetObjectRootRefsBatchResponse, error) {
	// Account for object root reads through the mounted World.
	started := time.Now()
	record := WorldStateOperationRecord{
		Name:              "GetObjectRootRefsBatch",
		StartKeyCount:     len(req.GetObjectKeys()),
		ResultObjectCount: 0,
	}
	var retErr error
	ctx, readCounter := block.WithReadCounter(ctx)
	defer func() {
		recordBlockReadSnapshot(&record, readCounter)
		r.observeOperation(record, started, retErr)
	}()

	// Read the requested object root references in one batch.
	refs, err := world.GetObjectRootRefsBatch(ctx, r.ws, req.GetObjectKeys())
	if err != nil {
		retErr = err
		return nil, err
	}

	// Encode root references and count objects present in the result.
	out := make([]*s4wave_world.ObjectRootRef, len(refs))
	for i, ref := range refs {
		if ref.Exists {
			record.ResultObjectCount++
		}
		out[i] = &s4wave_world.ObjectRootRef{
			ObjectKey: ref.ObjectKey,
			RootRef:   ref.RootRef,
			Rev:       ref.Rev,
			Exists:    ref.Exists,
		}
	}

	return &s4wave_world.GetObjectRootRefsBatchResponse{RootRefs: out}, nil
}

// GetObjectMetadataBatch returns graph metadata for object keys.
func (r *WorldStateResource) GetObjectMetadataBatch(ctx context.Context, req *s4wave_world.GetObjectMetadataBatchRequest) (*s4wave_world.GetObjectMetadataBatchResponse, error) {
	// Read graph metadata for the requested objects.
	metadata, err := world_types.GetObjectMetadataBatch(ctx, r.ws, req.GetObjectKeys())
	if err != nil {
		return nil, err
	}

	// Encode the indexed object metadata for the Resource client.
	out := make([]*s4wave_world.ObjectMetadata, len(metadata))
	for i, md := range metadata {
		out[i] = &s4wave_world.ObjectMetadata{
			ObjectKey:       md.ObjectKey,
			TypeId:          md.TypeID,
			ParentObjectKey: md.ParentObjectKey,
		}
	}

	return &s4wave_world.GetObjectMetadataBatchResponse{Metadata: out}, nil
}

// GetObjectBodiesBatch streams serialized object bodies for object keys.
//
// Each response is one page within the encoded body budget, in request
// order, with the World seqno the page was read at. The stream ends after the
// page holding the last key.
func (r *WorldStateResource) GetObjectBodiesBatch(
	req *s4wave_world.GetObjectBodiesBatchRequest,
	strm s4wave_world.SRPCWorldStateResourceService_GetObjectBodiesBatchStream,
) error {
	// Account for object body reads throughout the streaming request.
	started := time.Now()
	record := WorldStateOperationRecord{
		Name:          "GetObjectBodiesBatch",
		StartKeyCount: len(req.GetObjectKeys()),
	}
	var retErr error
	ctx, readCounter := block.WithReadCounter(strm.Context())
	defer func() {
		recordBlockReadSnapshot(&record, readCounter)
		r.observeOperation(record, started, retErr)
	}()

	// Stream object body pages and count existing objects.
	retErr = streamObjectBodyPages(
		ctx,
		r.ws,
		req.GetObjectKeys(),
		objectBodiesBatchBudget,
		func(resp *s4wave_world.GetObjectBodiesBatchResponse) error {
			for _, body := range resp.GetBodies() {
				if body.GetExists() {
					record.ResultObjectCount++
				}
			}
			return strm.Send(resp)
		},
	)
	return retErr
}

// objectBodiesBatchBudget bounds encoded object bodies in each Resource response.
const objectBodiesBatchBudget = world.ObjectBodiesBatchByteBudget

// streamObjectBodyPages reads keys in pages of at most bodyBudget encoded
// body bytes and passes each page to send in request order.
func streamObjectBodyPages(
	ctx context.Context,
	ws world.WorldState,
	keys []string,
	bodyBudget int,
	send func(*s4wave_world.GetObjectBodiesBatchResponse) error,
) error {
	for start := 0; start < len(keys); {
		bodies, consumed, worldSeqno, err := world.GetObjectBodiesBatchPageWithSeqno(ctx, ws, keys[start:], bodyBudget)
		if err != nil {
			return err
		}

		out := make([]*s4wave_world.ObjectBody, len(bodies))
		for i, body := range bodies {
			out[i] = &s4wave_world.ObjectBody{
				ObjectKey: body.ObjectKey,
				Body:      body.Body,
				Exists:    body.Exists,
				Rev:       body.Rev,
			}
		}
		err = send(&s4wave_world.GetObjectBodiesBatchResponse{
			Bodies:     out,
			WorldSeqno: worldSeqno,
		})
		if err != nil {
			return err
		}

		if consumed == 0 {
			return nil
		}
		start += int(consumed)
	}
	return nil
}

// QueryGraphPath creates a resource for a bounded graph path query.
func (r *WorldStateResource) QueryGraphPath(ctx context.Context, req *s4wave_world.QueryGraphPathRequest) (*s4wave_world.QueryGraphPathResponse, error) {
	// Account for the graph path query and its returned Resource.
	started := time.Now()
	record := WorldStateOperationRecord{
		Name:          "QueryGraphPath",
		StartKeyCount: len(req.GetStartKeys()),
		StepCount:     len(req.GetSteps()),
		Limit:         int(req.GetResultLimit()),
		PageSize:      int(req.GetPageSize()),
	}
	var retErr error
	ctx, readCounter := block.WithReadCounter(ctx)
	defer func() {
		recordBlockReadSnapshot(&record, readCounter)
		r.observeOperation(record, started, retErr)
	}()

	// Require the Resource client that will own the graph path result.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	// Validate the requested graph traversal steps.
	query, err := graphPathQueryFromProto(req)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	// Resolve the bounded graph path against the mounted World.
	result, err := r.ws.QueryGraphPath(ctx, query)
	if err != nil {
		retErr = err
		return nil, retErr
	}
	record.ResultObjectCount = len(result.ObjectKeys)
	record.ResultQuadCount = len(result.Quads)

	// Transfer graph result cleanup to the Resource client.
	queryResource := NewGraphPathQueryResource(r.le, r.b, result, req.GetPageSize())
	id, err := resourceCtx.AddResource(queryResource.GetMux(), func() {
		_, _ = queryResource.Close(context.Background(), &s4wave_world.CloseGraphPathQueryRequest{})
	})
	if err != nil {
		_, _ = queryResource.Close(ctx, &s4wave_world.CloseGraphPathQueryRequest{})
		retErr = err
		return nil, retErr
	}
	record.ResourceCreated = true

	return &s4wave_world.QueryGraphPathResponse{ResourceId: id}, nil
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
	// Report an unavailable World operation resolver.
	if r.lookupOp == nil {
		return &s4wave_world.ApplyWorldOpResponse{
			ErrorCode: s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP,
		}, nil
	}

	// Resolve the requested World operation and report unsupported types.
	opTypeID := req.GetOpTypeId()
	op, err := r.lookupOp(ctx, opTypeID)
	if err == nil && op == nil {
		err = world.ErrUnhandledOp
	}
	if err != nil {
		if errors.Is(err, world.ErrUnhandledOp) {
			return &s4wave_world.ApplyWorldOpResponse{
				ErrorCode: s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP,
			}, nil
		}
		return nil, err
	}

	// Decode the World operation payload before execution.
	err = op.UnmarshalBlock(req.GetOpData())
	if err != nil {
		return nil, err
	}

	// Bind the operation sender to the authenticated mount when available.
	opSender := r.sessionPeerID
	if !r.sessionPeerIDBound {
		opSender, err = req.ParsePeerID()
		if err != nil {
			return nil, err
		}
	}

	// Apply the World operation and encode any domain rejection.
	seqno, sysErr, err := r.ws.ApplyWorldOp(ctx, op, opSender)
	if err != nil {
		var rejection *world.OperationRejection
		if errors.As(err, &rejection) {
			return &s4wave_world.ApplyWorldOpResponse{
				RejectionCode: rejection.Code, RejectionMessage: rejection.Message,
			}, nil
		}

		if errors.Is(err, world.ErrUnhandledOp) {
			return &s4wave_world.ApplyWorldOpResponse{
				ErrorCode: s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP,
			}, nil
		}
		return nil, err
	}

	return &s4wave_world.ApplyWorldOpResponse{Seqno: seqno, SysErr: sysErr}, nil
}

// graphQuadsToProto converts graph quads to protobuf quads.
func graphQuadsToProto(quads []world.GraphQuad) []*quad.Quad {
	protoQuads := make([]*quad.Quad, len(quads))
	for i, q := range quads {
		protoQuads[i] = &quad.Quad{
			Subject:   q.GetSubject(),
			Predicate: q.GetPredicate(),
			Obj:       q.GetObj(),
			Label:     q.GetLabel(),
		}
	}
	return protoQuads
}

// validateBatchGraphFilter rejects broad remote graph scans.
func validateBatchGraphFilter(f *quad.Quad) error {
	if f.GetPredicate() == "" {
		return errors.New("batch graph filter predicate must be set")
	}
	if f.GetSubject() == "" && f.GetObj() == "" {
		return errors.New("batch graph filter subject or object must be set")
	}
	return nil
}

// observeOperation reports block reads and completion to the mount observer.
func (r *WorldStateResource) observeOperation(record WorldStateOperationRecord, started time.Time, err error) {
	if r.operationObserver == nil {
		return
	}
	record.Duration = time.Since(started)
	if err != nil {
		record.Error = err.Error()
	}
	r.operationObserver(record)
}

// recordBlockReadSnapshot copies the counter's snapshot into the record.
func recordBlockReadSnapshot(record *WorldStateOperationRecord, counter *block.ReadCounter) {
	record.ReadCounterSnapshot = counter.Snapshot()
}

// graphPathQueryFromProto validates traversal directions while decoding the query.
func graphPathQueryFromProto(req *s4wave_world.QueryGraphPathRequest) (*world.GraphPathQuery, error) {
	query := &world.GraphPathQuery{
		StartKeys:    req.GetStartKeys(),
		ResultLimit:  req.GetResultLimit(),
		IncludeQuads: req.GetIncludeQuads(),
	}
	query.Steps = make([]world.GraphPathStep, len(req.GetSteps()))
	for i, step := range req.GetSteps() {
		dir, err := graphPathDirectionFromProto(step.GetDirection())
		if err != nil {
			return nil, err
		}
		query.Steps[i] = world.GraphPathStep{
			Direction: dir,
			Predicate: step.GetPredicate(),
			Limit:     step.GetLimit(),
		}
	}
	return query, nil
}

// graphPathDirectionFromProto rejects unsupported traversal directions.
func graphPathDirectionFromProto(dir s4wave_world.GraphPathDirection) (world.GraphPathDirection, error) {
	switch dir {
	case s4wave_world.GraphPathDirection_GRAPH_PATH_DIRECTION_OUT:
		return world.GraphPathDirectionOut, nil
	case s4wave_world.GraphPathDirection_GRAPH_PATH_DIRECTION_IN:
		return world.GraphPathDirectionIn, nil
	case s4wave_world.GraphPathDirection_GRAPH_PATH_DIRECTION_BOTH:
		return world.GraphPathDirectionBoth, nil
	default:
		return 0, world.ErrGraphPathDirection
	}
}

// graphEdgeBucketDirectionFromProto decodes the indexed edge direction.
func graphEdgeBucketDirectionFromProto(dir s4wave_world.GraphEdgeBucketDirection) (world.GraphEdgeBucketDirection, error) {
	switch dir {
	case s4wave_world.GraphEdgeBucketDirection_GRAPH_EDGE_BUCKET_DIRECTION_UNSPECIFIED,
		s4wave_world.GraphEdgeBucketDirection_GRAPH_EDGE_BUCKET_DIRECTION_BOTH:
		return world.GraphEdgeBucketDirectionBoth, nil
	case s4wave_world.GraphEdgeBucketDirection_GRAPH_EDGE_BUCKET_DIRECTION_OUT:
		return world.GraphEdgeBucketDirectionOut, nil
	case s4wave_world.GraphEdgeBucketDirection_GRAPH_EDGE_BUCKET_DIRECTION_IN:
		return world.GraphEdgeBucketDirectionIn, nil
	default:
		return 0, world.ErrGraphEdgeBucketDirection
	}
}

// newObjectStateResource carries the World capability's authenticated sender to its objects.
func (r *WorldStateResource) newObjectStateResource(obj world.ObjectState) *ObjectStateResource {
	resource := NewObjectStateResource(r.le, r.b, obj, r.lookupOp)
	resource.sessionPeerID = r.sessionPeerID
	resource.sessionPeerIDBound = r.sessionPeerIDBound
	return resource
}

// _ is a type assertion.
var _ s4wave_world.SRPCWorldStateResourceServiceServer = (*WorldStateResource)(nil)
