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
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
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

	sessionPeerID      peer.ID
	sessionPeerIDBound bool
	operationObserver  WorldStateOperationObserver
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

func newWorldStateResource(
	le *logrus.Entry,
	b bus.Bus,
	ws world.WorldState,
	lookupOp world.LookupOp,
	engine world.Engine,
	opts ...WorldStateResourceOption,
) *WorldStateResource {
	wsResource := &WorldStateResource{le: le, b: b, ws: ws, lookupOp: lookupOp}
	applyWorldStateResourceOptions(wsResource, opts...)
	register := []func(srpc.Mux) error{
		func(mux srpc.Mux) error {
			return s4wave_world.SRPCRegisterWorldStateResourceService(mux, wsResource)
		},
	}
	if engine != nil {
		typedResource := newTypedObjectResourceWithSessionPeerID(
			le,
			b,
			ws,
			engine,
			wsResource.sessionPeerID,
			wsResource.sessionPeerIDBound,
		)
		register = append(register, func(mux srpc.Mux) error {
			return s4wave_world.SRPCRegisterTypedObjectResourceService(mux, typedResource)
		})
	}
	mux := resource_server.NewResourceMux(register...)
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

	owned, err := r.ws.BuildOwnedLookupCursor(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	cursorResource := resource_bucket_lookup.NewOwnedBucketLookupCursorResource(r.le, r.b, owned)
	id, err := resourceCtx.AddResource(cursorResource.GetMux(), owned.Release)
	if err != nil {
		owned.Release()
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

	return &s4wave_world.LookupGraphQuadsResponse{Quads: graphQuadsToProto(quads)}, nil
}

// LookupGraphQuadsBatch searches for graph quads using bounded indexed filters.
func (r *WorldStateResource) LookupGraphQuadsBatch(ctx context.Context, req *s4wave_world.LookupGraphQuadsBatchRequest) (*s4wave_world.LookupGraphQuadsBatchResponse, error) {
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

	if req.GetLimitPerFilter() == 0 {
		retErr = errors.New("limit_per_filter must be non-zero")
		return nil, retErr
	}

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

	direction, err := graphEdgeBucketDirectionFromProto(req.GetDirection())
	if err != nil {
		retErr = err
		return nil, retErr
	}

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

	refs, err := world.GetObjectRootRefsBatch(ctx, r.ws, req.GetObjectKeys())
	if err != nil {
		retErr = err
		return nil, err
	}

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
	metadata, err := world_types.GetObjectMetadataBatch(ctx, r.ws, req.GetObjectKeys())
	if err != nil {
		return nil, err
	}

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

// GetObjectBodiesBatch returns serialized object bodies for object keys.
func (r *WorldStateResource) GetObjectBodiesBatch(ctx context.Context, req *s4wave_world.GetObjectBodiesBatchRequest) (*s4wave_world.GetObjectBodiesBatchResponse, error) {
	started := time.Now()
	record := WorldStateOperationRecord{
		Name:              "GetObjectBodiesBatch",
		StartKeyCount:     len(req.GetObjectKeys()),
		ResultObjectCount: 0,
	}
	var retErr error
	ctx, readCounter := block.WithReadCounter(ctx)
	defer func() {
		recordBlockReadSnapshot(&record, readCounter)
		r.observeOperation(record, started, retErr)
	}()

	keys := req.GetObjectKeys()
	var bodies []*world.ObjectBody
	var nextKeyIndex uint32
	var worldSeqno uint64
	startKeyIndex := req.GetStartKeyIndex()
	if uint64(startKeyIndex) < uint64(len(keys)) {
		bodies, nextKeyIndex, worldSeqno, retErr = getObjectBodiesBatchPage(
			ctx,
			r.ws,
			keys,
			startKeyIndex,
			objectBodiesBatchBudget,
		)
		if retErr != nil {
			return nil, retErr
		}
	}

	out := make([]*s4wave_world.ObjectBody, len(bodies))
	for i, body := range bodies {
		if body.Exists {
			record.ResultObjectCount++
		}
		out[i] = &s4wave_world.ObjectBody{
			ObjectKey: body.ObjectKey,
			Body:      body.Body,
			Exists:    body.Exists,
			Rev:       body.Rev,
		}
	}

	return &s4wave_world.GetObjectBodiesBatchResponse{
		Bodies:       out,
		NextKeyIndex: nextKeyIndex,
		WorldSeqno:   worldSeqno,
	}, nil
}

const objectBodiesBatchBudget = world.ObjectBodiesBatchByteBudget

func getObjectBodiesBatchPage(
	ctx context.Context,
	ws world.WorldState,
	keys []string,
	startKeyIndex uint32,
	bodyBudget int,
) ([]*world.ObjectBody, uint32, uint64, error) {
	if uint64(startKeyIndex) >= uint64(len(keys)) {
		return nil, 0, 0, nil
	}
	start := int(startKeyIndex)
	bodies, consumed, worldSeqno, err := world.GetObjectBodiesBatchPageWithSeqno(ctx, ws, keys[start:], bodyBudget)
	if err != nil {
		return nil, 0, 0, err
	}
	if consumed == 0 || uint64(consumed) >= uint64(len(keys)-start) {
		return bodies, 0, worldSeqno, nil
	}
	return bodies, startKeyIndex + consumed, worldSeqno, nil
}

// QueryGraphPath creates a resource for a bounded graph path query.
func (r *WorldStateResource) QueryGraphPath(ctx context.Context, req *s4wave_world.QueryGraphPathRequest) (*s4wave_world.QueryGraphPathResponse, error) {
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

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	query, err := graphPathQueryFromProto(req)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	result, err := r.ws.QueryGraphPath(ctx, query)
	if err != nil {
		retErr = err
		return nil, retErr
	}
	record.ResultObjectCount = len(result.ObjectKeys)
	record.ResultQuadCount = len(result.Quads)

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
	if r.lookupOp == nil {
		return &s4wave_world.ApplyWorldOpResponse{
			ErrorCode: s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP,
		}, nil
	}

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

func recordBlockReadSnapshot(record *WorldStateOperationRecord, counter *block.ReadCounter) {
	snapshot := counter.Snapshot()
	record.BlockReadCount = snapshot.BlockReadCount
	record.BlockReadBytes = snapshot.BlockReadBytes
	record.BlockReadMissCount = snapshot.BlockReadMissCount
	record.ResourceGetBlockCount = snapshot.ResourceGetBlockCount
	record.ResourceGetBlockRefCount = snapshot.ResourceGetBlockRefCount
	record.ResourceGetBlockBytes = snapshot.ResourceGetBlockBytes
	record.ResourceGetBlockMissCount = snapshot.ResourceGetBlockMissCount
	record.DecodedBlockUnmarshalCount = snapshot.DecodedBlockUnmarshalCount
	record.DecodedBlockUnmarshalBytes = snapshot.DecodedBlockUnmarshalBytes
	record.DecodedBlockCacheAttemptCount = snapshot.DecodedBlockCacheAttemptCount
	record.DecodedBlockCacheHitCount = snapshot.DecodedBlockCacheHitCount
	record.DecodedBlockCacheMissCount = snapshot.DecodedBlockCacheMissCount
	record.DecodedBlockCloneCount = snapshot.DecodedBlockCloneCount
	record.DecodedBlockUncloneableCount = snapshot.DecodedBlockUncloneableCount
	record.DecodedBlockUncacheableCount = snapshot.DecodedBlockUncacheableCount
}

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

// _ is a type assertion
var _ s4wave_world.SRPCWorldStateResourceServiceServer = ((*WorldStateResource)(nil))
