package s4wave_world

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// WorldState represents the full state read/write interface to the world.
// WorldState implements all world state operations.
//
// In the Go implementation (hydra/world/world-state.go), WorldState provides:
// - GetReadOnly() bool
// - WorldStorage: BuildStorageCursor, AccessWorldState
// - WorldStateObject: CreateObject, GetObject, IterateObjects, RenameObject, DeleteObject
// - WorldStateGraph: SetGraphQuad, DeleteGraphQuad, LookupGraphQuads, DeleteGraphObject
// - WorldStateOp: ApplyWorldOp
// - WorldWaitSeqno: GetSeqno, WaitSeqno
//
// This Go SDK implementation wraps WorldStateResourceService.
type WorldState struct {
	client   *resource_client.Client
	ref      resource_client.ResourceRef
	service  SRPCWorldStateResourceServiceClient
	readOnly bool
}

// NewWorldState creates a new WorldState resource wrapper.
func NewWorldState(client *resource_client.Client, ref resource_client.ResourceRef, readOnly bool) (*WorldState, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return &WorldState{
		client:   client,
		ref:      ref,
		service:  NewSRPCWorldStateResourceServiceClient(srpcClient),
		readOnly: readOnly,
	}, nil
}

// GetResourceRef returns the resource reference.
func (ws *WorldState) GetResourceRef() resource_client.ResourceRef {
	return ws.ref
}

// Release releases the resource reference.
func (ws *WorldState) Release() {
	ws.ref.Release()
}

// GetReadOnly returns if the transaction is read-only.
// Returns stored metadata without RPC call.
func (ws *WorldState) GetReadOnly() bool {
	return ws.readOnly
}

// GetSeqno returns the current sequence number of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (ws *WorldState) GetSeqno(ctx context.Context) (uint64, error) {
	resp, err := ws.service.GetSeqno(ctx, &GetSeqnoRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Seqno, nil
}

// WaitSeqno waits for the world state sequence number to reach or exceed the specified value.
// Returns the seqno when the condition is reached.
// If seqno == 0, this might return immediately unconditionally.
func (ws *WorldState) WaitSeqno(ctx context.Context, seqno uint64) (uint64, error) {
	resp, err := ws.service.WaitSeqno(ctx, &WaitSeqnoRequest{Seqno: seqno})
	if err != nil {
		return 0, err
	}
	return resp.Seqno, nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the Tx.
// Returns the resource ID of the created cursor.
func (ws *WorldState) BuildStorageCursor(ctx context.Context) (uint32, error) {
	resp, err := ws.service.BuildStorageCursor(ctx, &BuildStorageCursorRequest{})
	if err != nil {
		return 0, err
	}
	return resp.ResourceId, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns a cursor pointing to the root world state.
// Returns the resource ID of the created cursor.
func (ws *WorldState) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef) (uint32, error) {
	resp, err := ws.service.AccessWorldState(ctx, &AccessWorldStateRequest{Ref: ref})
	if err != nil {
		return 0, err
	}
	return resp.ResourceId, nil
}

// CreateObject creates a new object in the world with the specified key and initial data.
// Returns ErrObjectExists if the object already exists.
// Appends a OBJECT_SET change to the changelog.
// Returns an ObjectState resource for the created object.
func (ws *WorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	resp, err := ws.service.CreateObject(ctx, &CreateObjectRequest{
		ObjectKey: key,
		RootRef:   rootRef,
	})
	if err != nil {
		return nil, err
	}

	objRef := ws.client.CreateResourceReference(resp.ResourceId)
	return NewObjectState(ws.client, objRef, resp.ObjectKey)
}

// GetObject retrieves an object from the world by its key.
// Returns (object, found, error).
func (ws *WorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	resp, err := ws.service.GetObject(ctx, &GetObjectRequest{ObjectKey: key})
	if err != nil {
		return nil, false, err
	}

	if !resp.Found {
		return nil, false, nil
	}

	objRef := ws.client.CreateResourceReference(resp.ResourceId)
	obj, err := NewObjectState(ws.client, objRef, resp.ObjectKey)
	if err != nil {
		objRef.Release()
		return nil, false, err
	}
	return obj, true, nil
}

// IterateObjects returns an iterator with the given object key prefix.
// The prefix is NOT clipped from the output keys.
// Keys are returned in sorted order.
// Returns the resource ID of the created iterator.
func (ws *WorldState) IterateObjects(ctx context.Context, prefix string, reversed bool) (uint32, error) {
	resp, err := ws.service.IterateObjects(ctx, &IterateObjectsRequest{
		Prefix:   prefix,
		Reversed: reversed,
	})
	if err != nil {
		return 0, err
	}
	return resp.ResourceId, nil
}

// RenameObject renames an object key and associated graph quads.
func (ws *WorldState) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	resp, err := ws.service.RenameObject(ctx, &RenameObjectRequest{
		OldObjectKey: oldKey,
		NewObjectKey: newKey,
		Descendants:  descendants,
	})
	if err != nil {
		return nil, err
	}

	objRef := ws.client.CreateResourceReference(resp.ResourceId)
	obj, err := NewObjectState(ws.client, objRef, resp.ObjectKey)
	if err != nil {
		objRef.Release()
		return nil, err
	}
	return obj, nil
}

// DeleteObject removes an object and all associated graph quads from the world.
// Calls DeleteGraphObject internally.
// Returns (deleted, error). deleted=false if not found.
func (ws *WorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	resp, err := ws.service.DeleteObject(ctx, &DeleteObjectRequest{ObjectKey: key})
	if err != nil {
		return false, err
	}
	return resp.Deleted, nil
}

// SetGraphQuad adds or updates a quad in the graph store.
// Subject: must be an existing object IRI: <object-key>
// Predicate: a predicate string, e.g. IRI: <ref>
// Object: an existing object IRI: <object-key>
// If already exists, returns nil.
func (ws *WorldState) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	protoQuad := &quad.Quad{
		Subject:   q.GetSubject(),
		Predicate: q.GetPredicate(),
		Obj:       q.GetObj(),
		Label:     q.GetLabel(),
	}
	_, err := ws.service.SetGraphQuad(ctx, &SetGraphQuadRequest{Quad: protoQuad})
	return err
}

// DeleteGraphQuad removes a specific quad from the graph store.
// Note: if quad did not exist, returns nil.
func (ws *WorldState) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	protoQuad := &quad.Quad{
		Subject:   q.GetSubject(),
		Predicate: q.GetPredicate(),
		Obj:       q.GetObj(),
		Label:     q.GetLabel(),
	}
	_, err := ws.service.DeleteGraphQuad(ctx, &DeleteGraphQuadRequest{Quad: protoQuad})
	return err
}

// LookupGraphQuads searches for graph quads matching the specified filter criteria.
// If the filter fields are empty, matches any for that field.
// If not found, returns empty list.
// If limit is set, stops after finding that number of matching quads.
func (ws *WorldState) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	protoFilter := &quad.Quad{
		Subject:   filter.GetSubject(),
		Predicate: filter.GetPredicate(),
		Obj:       filter.GetObj(),
		Label:     filter.GetLabel(),
	}
	resp, err := ws.service.LookupGraphQuads(ctx, &LookupGraphQuadsRequest{
		Filter: protoFilter,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}

	quads := make([]world.GraphQuad, len(resp.Quads))
	for i, q := range resp.Quads {
		quads[i] = q
	}
	return quads, nil
}

// LookupGraphQuadsBatch searches for graph quads using bounded indexed filters.
func (ws *WorldState) LookupGraphQuadsBatch(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	protoFilters := make([]*quad.Quad, len(filters))
	for i, filter := range filters {
		protoFilters[i] = &quad.Quad{
			Subject:   filter.GetSubject(),
			Predicate: filter.GetPredicate(),
			Obj:       filter.GetObj(),
			Label:     filter.GetLabel(),
		}
	}

	resp, err := ws.service.LookupGraphQuadsBatch(ctx, &LookupGraphQuadsBatchRequest{
		Filters:        protoFilters,
		LimitPerFilter: limitPerFilter,
	})
	if err != nil {
		return nil, err
	}

	results := make([][]world.GraphQuad, len(resp.GetResults()))
	for i, result := range resp.GetResults() {
		quads := make([]world.GraphQuad, len(result.GetQuads()))
		for j, q := range result.GetQuads() {
			quads[j] = q
		}
		results[i] = quads
	}
	return results, nil
}

// ListGraphEdgeBuckets lists grouped inbound/outbound graph edge buckets.
func (ws *WorldState) ListGraphEdgeBuckets(ctx context.Context, query *world.GraphEdgeBucketQuery) ([]*world.GraphEdgeBucket, error) {
	req := graphEdgeBucketQueryToProto(query)
	resp, err := ws.service.ListGraphEdgeBuckets(ctx, req)
	if err != nil {
		return nil, err
	}

	buckets := make([]*world.GraphEdgeBucket, len(resp.GetBuckets()))
	for i, bucket := range resp.GetBuckets() {
		outgoing := make([]world.GraphQuad, len(bucket.GetOutgoing()))
		for j, q := range bucket.GetOutgoing() {
			outgoing[j] = q
		}
		incoming := make([]world.GraphQuad, len(bucket.GetIncoming()))
		for j, q := range bucket.GetIncoming() {
			incoming[j] = q
		}
		buckets[i] = &world.GraphEdgeBucket{
			OriginObjectKey:   bucket.GetOriginObjectKey(),
			Outgoing:          outgoing,
			Incoming:          incoming,
			OutgoingTruncated: bucket.GetOutgoingTruncated(),
			IncomingTruncated: bucket.GetIncomingTruncated(),
		}
	}
	return buckets, nil
}

// ListObjectsWithType lists object keys with the given type identifier.
func (ws *WorldState) ListObjectsWithType(ctx context.Context, typeID string) ([]string, error) {
	resp, err := ws.service.ListObjectsWithType(ctx, &ListObjectsWithTypeRequest{
		TypeId: typeID,
	})
	if err != nil {
		return nil, err
	}
	return resp.ObjectKeys, nil
}

// GetObjectRootRefsBatch returns root references for object keys.
func (ws *WorldState) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*world.ObjectRootRef, error) {
	resp, err := ws.service.GetObjectRootRefsBatch(ctx, &GetObjectRootRefsBatchRequest{
		ObjectKeys: keys,
	})
	if err != nil {
		return nil, err
	}

	refs := make([]*world.ObjectRootRef, len(resp.GetRootRefs()))
	for i, ref := range resp.GetRootRefs() {
		refs[i] = &world.ObjectRootRef{
			ObjectKey: ref.GetObjectKey(),
			RootRef:   ref.GetRootRef().Clone(),
			Rev:       ref.GetRev(),
			Exists:    ref.GetExists(),
		}
	}
	return refs, nil
}

// GetObjectMetadataBatch returns graph metadata for object keys.
func (ws *WorldState) GetObjectMetadataBatch(ctx context.Context, keys []string) ([]*world_types.ObjectMetadata, error) {
	resp, err := ws.service.GetObjectMetadataBatch(ctx, &GetObjectMetadataBatchRequest{
		ObjectKeys: keys,
	})
	if err != nil {
		return nil, err
	}

	metadata := make([]*world_types.ObjectMetadata, len(resp.GetMetadata()))
	for i, md := range resp.GetMetadata() {
		metadata[i] = &world_types.ObjectMetadata{
			ObjectKey:       md.GetObjectKey(),
			TypeID:          md.GetTypeId(),
			ParentObjectKey: md.GetParentObjectKey(),
		}
	}
	return metadata, nil
}

// ForEachObjectBodyPage calls cb with each page of serialized object bodies.
// The callback must finish with the page before the next RPC response is read.
func (ws *WorldState) ForEachObjectBodyPage(
	ctx context.Context,
	keys []string,
	cb func([]*world.ObjectBody) error,
) error {
	return ForEachObjectBodyPage(ctx, ws.service, keys, cb)
}

// GetObjectBodiesBatch returns serialized object bodies for object keys.
func (ws *WorldState) GetObjectBodiesBatch(ctx context.Context, keys []string) ([]*world.ObjectBody, error) {
	return GetObjectBodiesBatch(ctx, ws.service, keys)
}

// QueryGraphPath executes a bounded server-side graph path query.
func (ws *WorldState) QueryGraphPath(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	req, err := graphPathQueryToProto(query)
	if err != nil {
		return nil, err
	}
	resp, err := ws.service.QueryGraphPath(ctx, req)
	if err != nil {
		return nil, err
	}

	ref := ws.client.CreateResourceReference(resp.GetResourceId())
	defer ref.Release()
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	service := NewSRPCGraphPathQueryResourceServiceClient(srpcClient)
	defer service.Close(ctx, &CloseGraphPathQueryRequest{})

	result := &world.GraphPathQueryResult{}
	for {
		page, err := service.Next(ctx, &NextGraphPathQueryRequest{})
		if err != nil {
			return nil, err
		}
		result.ObjectKeys = append(result.ObjectKeys, page.GetObjectKeys()...)
		for _, q := range page.GetQuads() {
			result.Quads = append(result.Quads, q)
		}
		if page.GetDone() {
			return result, nil
		}
	}
}

// DeleteGraphObject removes all graph quads that reference the specified object key.
// Note: objectKey should be the object key, NOT the object key <iri> format.
func (ws *WorldState) DeleteGraphObject(ctx context.Context, objectKey string) error {
	_, err := ws.service.DeleteGraphObject(ctx, &DeleteGraphObjectRequest{ObjectKey: objectKey})
	return err
}

// ApplyWorldOp applies a batch operation at the world level.
// The handling of the operation is operation-type specific.
// Returns (seqno, sysErr, err).
// If nil is returned for the error, implies success.
// If sysErr is set, the error is treated as a transient system error.
func (ws *WorldState) ApplyWorldOp(ctx context.Context, opTypeID string, opData []byte, opSender string) (uint64, bool, error) {
	resp, err := ws.service.ApplyWorldOp(ctx, &ApplyWorldOpRequest{
		OpTypeId: opTypeID,
		OpData:   opData,
		OpSender: opSender,
	})
	if err != nil {
		return 0, false, err
	}
	if err := ErrorFromCode(resp.GetErrorCode()); err != nil {
		return 0, false, err
	}
	return resp.Seqno, resp.SysErr, nil
}

func graphPathQueryToProto(query *world.GraphPathQuery) (*QueryGraphPathRequest, error) {
	if query == nil {
		return &QueryGraphPathRequest{}, nil
	}
	steps := make([]*GraphPathStep, len(query.Steps))
	for i, step := range query.Steps {
		dir, err := graphPathDirectionToProto(step.Direction)
		if err != nil {
			return nil, err
		}
		steps[i] = &GraphPathStep{
			Direction: dir,
			Predicate: step.Predicate,
			Limit:     step.Limit,
		}
	}
	return &QueryGraphPathRequest{
		StartKeys:    query.StartKeys,
		Steps:        steps,
		ResultLimit:  query.ResultLimit,
		IncludeQuads: query.IncludeQuads,
		PageSize:     query.ResultLimit,
	}, nil
}

func graphPathDirectionToProto(dir world.GraphPathDirection) (GraphPathDirection, error) {
	switch dir {
	case world.GraphPathDirectionOut:
		return GraphPathDirection_GRAPH_PATH_DIRECTION_OUT, nil
	case world.GraphPathDirectionIn:
		return GraphPathDirection_GRAPH_PATH_DIRECTION_IN, nil
	case world.GraphPathDirectionBoth:
		return GraphPathDirection_GRAPH_PATH_DIRECTION_BOTH, nil
	default:
		return 0, world.ErrGraphPathDirection
	}
}

func graphEdgeBucketQueryToProto(query *world.GraphEdgeBucketQuery) *ListGraphEdgeBucketsRequest {
	if query == nil {
		return &ListGraphEdgeBucketsRequest{}
	}
	return &ListGraphEdgeBucketsRequest{
		OriginObjectKeys: query.OriginObjectKeys,
		Predicate:        query.Predicate,
		LimitPerOrigin:   query.LimitPerOrigin,
		Direction:        graphEdgeBucketDirectionToProto(query.Direction),
	}
}

func graphEdgeBucketDirectionToProto(dir world.GraphEdgeBucketDirection) GraphEdgeBucketDirection {
	switch dir {
	case world.GraphEdgeBucketDirectionOut:
		return GraphEdgeBucketDirection_GRAPH_EDGE_BUCKET_DIRECTION_OUT
	case world.GraphEdgeBucketDirectionIn:
		return GraphEdgeBucketDirection_GRAPH_EDGE_BUCKET_DIRECTION_IN
	case world.GraphEdgeBucketDirectionBoth:
		return GraphEdgeBucketDirection_GRAPH_EDGE_BUCKET_DIRECTION_BOTH
	default:
		return GraphEdgeBucketDirection_GRAPH_EDGE_BUCKET_DIRECTION_UNSPECIFIED
	}
}
