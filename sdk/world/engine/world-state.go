package sdk_world_engine

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// SDKWorldState implements world.WorldState over SRPC by delegating to
// WorldStateResourceService calls on a remote resource.
type SDKWorldState struct {
	client   ResourceClient
	ref      resource_client.ResourceRef
	service  s4wave_world.SRPCWorldStateResourceServiceClient
	readOnly bool
}

// NewSDKWorldState creates a new SDKWorldState wrapping a resource reference.
func NewSDKWorldState(client ResourceClient, ref resource_client.ResourceRef, readOnly bool) (*SDKWorldState, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return &SDKWorldState{
		client:   client,
		ref:      ref,
		service:  s4wave_world.NewSRPCWorldStateResourceServiceClient(srpcClient),
		readOnly: readOnly,
	}, nil
}

// Release releases the underlying resource reference.
func (ws *SDKWorldState) Release() {
	ws.ref.Release()
}

// GetReadOnly returns if the state is read-only.
func (ws *SDKWorldState) GetReadOnly() bool {
	return ws.readOnly
}

// GetSeqno returns the current sequence number of the world state.
func (ws *SDKWorldState) GetSeqno(ctx context.Context) (uint64, error) {
	resp, err := ws.service.GetSeqno(ctx, &s4wave_world.GetSeqnoRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Seqno, nil
}

// Sync fences the block writes made through this world state durable.
func (ws *SDKWorldState) Sync(ctx context.Context) (bool, error) {
	resp, err := ws.service.Sync(ctx, &s4wave_world.SyncRequest{})
	if err != nil {
		return false, err
	}
	return resp.GetFenced(), nil
}

// WaitSeqno waits for the world state sequence number to reach or exceed the specified value.
func (ws *SDKWorldState) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	resp, err := ws.service.WaitSeqno(ctx, &s4wave_world.WaitSeqnoRequest{Seqno: value})
	if err != nil {
		return 0, err
	}
	return resp.Seqno, nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
func (ws *SDKWorldState) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	resp, err := ws.service.BuildStorageCursor(ctx, &s4wave_world.BuildStorageCursorRequest{})
	if err != nil {
		return nil, err
	}

	ref := ws.client.CreateResourceReference(resp.GetResourceId())
	cursor, err := newSDKBucketLookupCursor(ctx, ref)
	if err != nil {
		ref.Release()
		return nil, err
	}
	return cursor, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
func (ws *SDKWorldState) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	resp, err := ws.service.AccessWorldState(ctx, &s4wave_world.AccessWorldStateRequest{Ref: ref})
	if err != nil {
		return err
	}
	return accessSDKBucketLookupCursor(ctx, ws.client, resp.GetResourceId(), cb)
}

// CreateObject creates an object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
func (ws *SDKWorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	resp, err := ws.service.CreateObject(ctx, &s4wave_world.CreateObjectRequest{
		ObjectKey: key,
		RootRef:   rootRef,
	})
	if err != nil {
		return nil, err
	}

	objRef := ws.client.CreateResourceReference(resp.ResourceId)
	obj, err := NewSDKObjectState(ws.client, objRef, resp.ObjectKey)
	if err != nil {
		objRef.Release()
		return nil, err
	}
	return obj, nil
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (ws *SDKWorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	resp, err := ws.service.GetObject(ctx, &s4wave_world.GetObjectRequest{ObjectKey: key})
	if err != nil {
		return nil, false, err
	}

	if !resp.Found {
		return nil, false, nil
	}

	objRef := ws.client.CreateResourceReference(resp.ResourceId)
	obj, err := NewSDKObjectState(ws.client, objRef, resp.ObjectKey)
	if err != nil {
		objRef.Release()
		return nil, false, err
	}
	return obj, true, nil
}

// IterateObjects returns an iterator with the given object key prefix.
// The prefix is NOT clipped from the output keys.
// Keys are returned in sorted order.
// Must call Next() or Seek() before valid.
// Call Close when done with the iterator.
func (ws *SDKWorldState) IterateObjects(ctx context.Context, prefix string, reversed bool) world.ObjectIterator {
	resp, err := ws.service.IterateObjects(ctx, &s4wave_world.IterateObjectsRequest{
		Prefix:   prefix,
		Reversed: reversed,
	})
	if err != nil {
		return &SDKObjectIterator{ctx: ctx, err: err}
	}

	iterRef := ws.client.CreateResourceReference(resp.ResourceId)
	iter, iterErr := NewSDKObjectIterator(ctx, iterRef)
	if iterErr != nil {
		iterRef.Release()
		return &SDKObjectIterator{ctx: ctx, err: iterErr}
	}
	return iter
}

// RenameObject renames an object key and updates associated graph quads.
func (ws *SDKWorldState) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	resp, err := ws.service.RenameObject(ctx, &s4wave_world.RenameObjectRequest{
		OldObjectKey: oldKey,
		NewObjectKey: newKey,
		Descendants:  descendants,
	})
	if err != nil {
		return nil, err
	}

	objRef := ws.client.CreateResourceReference(resp.ResourceId)
	obj, err := NewSDKObjectState(ws.client, objRef, resp.ObjectKey)
	if err != nil {
		objRef.Release()
		return nil, err
	}
	return obj, nil
}

// DeleteObject deletes an object and associated graph quads by ID.
// Returns false, nil if not found.
func (ws *SDKWorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	resp, err := ws.service.DeleteObject(ctx, &s4wave_world.DeleteObjectRequest{ObjectKey: key})
	if err != nil {
		return false, err
	}
	return resp.Deleted, nil
}

// AccessCayleyGraph rejects local Cayley handle access for remote worlds.
func (ws *SDKWorldState) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	return ErrRemoteCayleyGraphUnsupported
}

// SetGraphQuad sets a quad in the graph store.
func (ws *SDKWorldState) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	protoQuad := &quad.Quad{
		Subject:   q.GetSubject(),
		Predicate: q.GetPredicate(),
		Obj:       q.GetObj(),
		Label:     q.GetLabel(),
	}
	_, err := ws.service.SetGraphQuad(ctx, &s4wave_world.SetGraphQuadRequest{Quad: protoQuad})
	return err
}

// DeleteGraphQuad deletes a quad from the graph store.
func (ws *SDKWorldState) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	protoQuad := &quad.Quad{
		Subject:   q.GetSubject(),
		Predicate: q.GetPredicate(),
		Obj:       q.GetObj(),
		Label:     q.GetLabel(),
	}
	_, err := ws.service.DeleteGraphQuad(ctx, &s4wave_world.DeleteGraphQuadRequest{Quad: protoQuad})
	return err
}

// LookupGraphQuads searches for graph quads in the store.
func (ws *SDKWorldState) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	protoFilter := &quad.Quad{
		Subject:   filter.GetSubject(),
		Predicate: filter.GetPredicate(),
		Obj:       filter.GetObj(),
		Label:     filter.GetLabel(),
	}
	resp, err := ws.service.LookupGraphQuads(ctx, &s4wave_world.LookupGraphQuadsRequest{
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
func (ws *SDKWorldState) LookupGraphQuadsBatch(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	protoFilters := make([]*quad.Quad, len(filters))
	for i, filter := range filters {
		protoFilters[i] = &quad.Quad{
			Subject:   filter.GetSubject(),
			Predicate: filter.GetPredicate(),
			Obj:       filter.GetObj(),
			Label:     filter.GetLabel(),
		}
	}

	resp, err := ws.service.LookupGraphQuadsBatch(ctx, &s4wave_world.LookupGraphQuadsBatchRequest{
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
func (ws *SDKWorldState) ListGraphEdgeBuckets(ctx context.Context, query *world.GraphEdgeBucketQuery) ([]*world.GraphEdgeBucket, error) {
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
func (ws *SDKWorldState) ListObjectsWithType(ctx context.Context, typeID string) ([]string, error) {
	resp, err := ws.service.ListObjectsWithType(ctx, &s4wave_world.ListObjectsWithTypeRequest{
		TypeId: typeID,
	})
	if err != nil {
		return nil, err
	}
	return resp.ObjectKeys, nil
}

// GetObjectRootRefsBatch returns root references for object keys.
func (ws *SDKWorldState) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*world.ObjectRootRef, error) {
	resp, err := ws.service.GetObjectRootRefsBatch(ctx, &s4wave_world.GetObjectRootRefsBatchRequest{
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
func (ws *SDKWorldState) GetObjectMetadataBatch(ctx context.Context, keys []string) ([]*world_types.ObjectMetadata, error) {
	resp, err := ws.service.GetObjectMetadataBatch(ctx, &s4wave_world.GetObjectMetadataBatchRequest{
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

// QueryGraphPath executes a bounded server-side graph path query.
func (ws *SDKWorldState) QueryGraphPath(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
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
	service := s4wave_world.NewSRPCGraphPathQueryResourceServiceClient(srpcClient)
	defer service.Close(ctx, &s4wave_world.CloseGraphPathQueryRequest{})

	result := &world.GraphPathQueryResult{}
	for {
		page, err := service.Next(ctx, &s4wave_world.NextGraphPathQueryRequest{})
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

// DeleteGraphObject deletes all quads with Subject or Object set to value.
func (ws *SDKWorldState) DeleteGraphObject(ctx context.Context, value string) error {
	_, err := ws.service.DeleteGraphObject(ctx, &s4wave_world.DeleteGraphObjectRequest{ObjectKey: value})
	return err
}

// ApplyWorldOp applies a batch operation at the world level.
// The handling of the operation is operation-type specific.
// Returns seqno, sysErr, err.
func (ws *SDKWorldState) ApplyWorldOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	opData, err := op.MarshalBlock()
	if err != nil {
		return 0, false, err
	}

	resp, err := ws.service.ApplyWorldOp(ctx, &s4wave_world.ApplyWorldOpRequest{
		OpTypeId: op.GetOperationTypeId(),
		OpData:   opData,
		OpSender: sender.String(),
	})
	if err != nil {
		return 0, false, err
	}
	return resp.Seqno, resp.SysErr, nil
}

// _ is a type assertion
var _ world.WorldState = (*SDKWorldState)(nil)

func graphPathQueryToProto(query *world.GraphPathQuery) (*s4wave_world.QueryGraphPathRequest, error) {
	if query == nil {
		return &s4wave_world.QueryGraphPathRequest{}, nil
	}
	steps := make([]*s4wave_world.GraphPathStep, len(query.Steps))
	for i, step := range query.Steps {
		dir, err := graphPathDirectionToProto(step.Direction)
		if err != nil {
			return nil, err
		}
		steps[i] = &s4wave_world.GraphPathStep{
			Direction: dir,
			Predicate: step.Predicate,
			Limit:     step.Limit,
		}
	}
	return &s4wave_world.QueryGraphPathRequest{
		StartKeys:    query.StartKeys,
		Steps:        steps,
		ResultLimit:  query.ResultLimit,
		IncludeQuads: query.IncludeQuads,
		PageSize:     query.ResultLimit,
	}, nil
}

func graphPathDirectionToProto(dir world.GraphPathDirection) (s4wave_world.GraphPathDirection, error) {
	switch dir {
	case world.GraphPathDirectionOut:
		return s4wave_world.GraphPathDirection_GRAPH_PATH_DIRECTION_OUT, nil
	case world.GraphPathDirectionIn:
		return s4wave_world.GraphPathDirection_GRAPH_PATH_DIRECTION_IN, nil
	case world.GraphPathDirectionBoth:
		return s4wave_world.GraphPathDirection_GRAPH_PATH_DIRECTION_BOTH, nil
	default:
		return 0, world.ErrGraphPathDirection
	}
}

func graphEdgeBucketQueryToProto(query *world.GraphEdgeBucketQuery) *s4wave_world.ListGraphEdgeBucketsRequest {
	if query == nil {
		return &s4wave_world.ListGraphEdgeBucketsRequest{}
	}
	return &s4wave_world.ListGraphEdgeBucketsRequest{
		OriginObjectKeys: query.OriginObjectKeys,
		Predicate:        query.Predicate,
		LimitPerOrigin:   query.LimitPerOrigin,
		Direction:        graphEdgeBucketDirectionToProto(query.Direction),
	}
}

func graphEdgeBucketDirectionToProto(dir world.GraphEdgeBucketDirection) s4wave_world.GraphEdgeBucketDirection {
	switch dir {
	case world.GraphEdgeBucketDirectionOut:
		return s4wave_world.GraphEdgeBucketDirection_GRAPH_EDGE_BUCKET_DIRECTION_OUT
	case world.GraphEdgeBucketDirectionIn:
		return s4wave_world.GraphEdgeBucketDirection_GRAPH_EDGE_BUCKET_DIRECTION_IN
	case world.GraphEdgeBucketDirectionBoth:
		return s4wave_world.GraphEdgeBucketDirection_GRAPH_EDGE_BUCKET_DIRECTION_BOTH
	default:
		return s4wave_world.GraphEdgeBucketDirection_GRAPH_EDGE_BUCKET_DIRECTION_UNSPECIFIED
	}
}
