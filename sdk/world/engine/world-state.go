package sdk_world_engine

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_bucket_lookup "github.com/s4wave/spacewave/sdk/bucket/lookup"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// SDKWorldState implements world.WorldState over SRPC by delegating to
// WorldStateResourceService calls on a remote resource.
type SDKWorldState struct {
	// client creates references to resources returned by the service.
	client ResourceClient
	// ref retains the remote World state.
	ref resource_client.ResourceRef
	// service accesses the remote World state.
	service s4wave_world.SRPCWorldStateResourceServiceClient
	// readOnly is the transaction mode reported by the server.
	readOnly bool
}

// NewSDKWorldState creates a new SDKWorldState wrapping a resource reference.
func NewSDKWorldState(client ResourceClient, ref resource_client.ResourceRef, readOnly bool) (*SDKWorldState, error) {
	// Acquire the remote World state client.
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}

	// Retain the resource and its transaction mode.
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

// StageWorldState opens a remote stage on the World state.
// Releasing it releases the stage resource.
func (ws *SDKWorldState) StageWorldState(ctx context.Context) (world.WorldStage, error) {
	// Preserve the read-only error identity before contacting the server.
	if ws.GetReadOnly() {
		return nil, tx.ErrNotWrite
	}

	// Open a remote staging scope for this writable World state.
	resp, err := ws.service.StageWorldState(ctx, &s4wave_world.StageWorldStateRequest{})
	if err != nil {
		return nil, err
	}
	return newSDKStage(ws.client, resp.GetResourceId())
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
	// Request a storage cursor from the remote World state.
	resp, err := ws.service.BuildStorageCursor(ctx, &s4wave_world.BuildStorageCursorRequest{})
	if err != nil {
		return nil, err
	}

	// Wrap the remote cursor resource and release it if construction fails.
	ref := ws.client.CreateResourceReference(resp.GetResourceId())
	cursor, err := s4wave_bucket_lookup.NewCursor(ctx, ref)
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
	return s4wave_bucket_lookup.AccessCursor(ctx, ws.client, resp.GetResourceId(), cb)
}

// OpenNestedWorld opens a typed outer object's immutable sub-World.
// Release the returned state independently of this World state.
func (ws *SDKWorldState) OpenNestedWorld(ctx context.Context, key string) (*SDKWorldState, error) {
	// Open the immutable sub-World through its outer object.
	resp, err := ws.service.OpenNestedWorld(ctx, &s4wave_world.OpenNestedWorldRequest{ObjectKey: key})
	if err != nil {
		return nil, err
	}

	// Wrap the nested World resource with an independent reference.
	ref := ws.client.CreateResourceReference(resp.GetResourceId())
	nested, err := NewSDKWorldState(ws.client, ref, true)
	if err != nil {
		ref.Release()
		return nil, err
	}
	return nested, nil
}

// CreateObject creates an object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
func (ws *SDKWorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	// Preserve the read-only error identity before contacting the server.
	if ws.GetReadOnly() {
		return nil, tx.ErrNotWrite
	}

	// Create the remote World object with its initial root reference.
	resp, err := ws.service.CreateObject(ctx, &s4wave_world.CreateObjectRequest{
		ObjectKey: key,
		RootRef:   rootRef,
	})
	if err != nil {
		return nil, err
	}

	// Wrap the created object resource and release it if construction fails.
	objRef := ws.client.CreateResourceReference(resp.ResourceId)
	obj, err := NewSDKObjectState(ws.client, objRef, resp.ObjectKey, ws.GetReadOnly())
	if err != nil {
		objRef.Release()
		return nil, err
	}
	return obj, nil
}

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (ws *SDKWorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	// Look up the object in the remote World state.
	resp, err := ws.service.GetObject(ctx, &s4wave_world.GetObjectRequest{ObjectKey: key})
	if err != nil {
		return nil, false, err
	}

	// Return a missing-object result without acquiring a resource reference.
	if !resp.Found {
		return nil, false, nil
	}

	// Wrap the found object resource and release it if construction fails.
	objRef := ws.client.CreateResourceReference(resp.ResourceId)
	obj, err := NewSDKObjectState(ws.client, objRef, resp.ObjectKey, ws.GetReadOnly())
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
	// Request a remote iterator over the matching World object keys.
	resp, err := ws.service.IterateObjects(ctx, &s4wave_world.IterateObjectsRequest{
		Prefix:   prefix,
		Reversed: reversed,
	})
	if err != nil {
		return &SDKObjectIterator{ctx: ctx, err: err}
	}

	// Wrap the iterator resource and retain construction failures on the iterator.
	iterRef := ws.client.CreateResourceReference(resp.ResourceId)
	iter, iterErr := NewSDKObjectIterator(ctx, iterRef)
	if iterErr != nil {
		iterRef.Release()
		return &SDKObjectIterator{ctx: ctx, err: iterErr}
	}
	return iter
}

// ListObjects returns one page of at most limit entries whose keys start with
// prefix, in key order, beginning after startAfter. A non-empty delimiter
// groups keys below the next path segment into prefixes. more reports that
// further entries match; pass the greater of the last object key and the last
// prefix as startAfter to read them.
func (ws *SDKWorldState) ListObjects(
	ctx context.Context,
	prefix string,
	delimiter string,
	startAfter string,
	limit uint32,
) (objects []*world_types.ObjectMetadata, prefixes []string, more bool, err error) {
	resp, err := ws.service.ListObjects(ctx, &s4wave_world.ListObjectsRequest{
		Prefix:     prefix,
		Delimiter:  delimiter,
		StartAfter: startAfter,
		Limit:      limit,
	})
	if err != nil {
		return nil, nil, false, err
	}
	return objectMetadataFromProto(resp.GetObjects()), resp.GetPrefixes(), resp.GetMore(), nil
}

// RenameObject renames an object key and updates associated graph quads.
func (ws *SDKWorldState) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	// Preserve the read-only error identity before contacting the server.
	if ws.GetReadOnly() {
		return nil, tx.ErrNotWrite
	}

	// Rename the remote World object and its associated graph quads.
	resp, err := ws.service.RenameObject(ctx, &s4wave_world.RenameObjectRequest{
		OldObjectKey: oldKey,
		NewObjectKey: newKey,
		Descendants:  descendants,
	})
	if err != nil {
		return nil, err
	}

	// Wrap the renamed object resource and release it if construction fails.
	objRef := ws.client.CreateResourceReference(resp.ResourceId)
	obj, err := NewSDKObjectState(ws.client, objRef, resp.ObjectKey, ws.GetReadOnly())
	if err != nil {
		objRef.Release()
		return nil, err
	}
	return obj, nil
}

// DeleteObject deletes an object and associated graph quads by ID.
// Returns false, nil if not found.
func (ws *SDKWorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	// Preserve the read-only error identity before contacting the server.
	if ws.GetReadOnly() {
		return false, tx.ErrNotWrite
	}

	// Delete the remote object and return whether it existed.
	resp, err := ws.service.DeleteObject(ctx, &s4wave_world.DeleteObjectRequest{ObjectKey: key})
	if err != nil {
		return false, err
	}
	return resp.Deleted, nil
}

// AccessCayleyGraph rejects local Cayley handle access for remote worlds.
func (ws *SDKWorldState) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	// Preserve the read-only error identity for requested writes.
	if write && ws.GetReadOnly() {
		return tx.ErrNotWrite
	}

	// Remote Worlds cannot expose a local Cayley handle.
	return ErrRemoteCayleyGraphUnsupported
}

// SetGraphQuad sets a quad in the graph store.
func (ws *SDKWorldState) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	// Preserve the read-only error identity before contacting the server.
	if ws.GetReadOnly() {
		return tx.ErrNotWrite
	}

	// Encode the World graph quad for the resource service.
	protoQuad := &quad.Quad{
		Subject:   q.GetSubject(),
		Predicate: q.GetPredicate(),
		Obj:       q.GetObj(),
		Label:     q.GetLabel(),
	}

	// Store the encoded quad in the remote World graph.
	_, err := ws.service.SetGraphQuad(ctx, &s4wave_world.SetGraphQuadRequest{Quad: protoQuad})
	return err
}

// DeleteGraphQuad deletes a quad from the graph store.
func (ws *SDKWorldState) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	// Preserve the read-only error identity before contacting the server.
	if ws.GetReadOnly() {
		return tx.ErrNotWrite
	}

	// Encode the World graph quad for the resource service.
	protoQuad := &quad.Quad{
		Subject:   q.GetSubject(),
		Predicate: q.GetPredicate(),
		Obj:       q.GetObj(),
		Label:     q.GetLabel(),
	}

	// Delete the encoded quad in the remote World graph.
	_, err := ws.service.DeleteGraphQuad(ctx, &s4wave_world.DeleteGraphQuadRequest{Quad: protoQuad})
	return err
}

// LookupGraphQuads searches for graph quads in the store.
func (ws *SDKWorldState) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	// Encode the graph filter for the resource service.
	protoFilter := &quad.Quad{
		Subject:   filter.GetSubject(),
		Predicate: filter.GetPredicate(),
		Obj:       filter.GetObj(),
		Label:     filter.GetLabel(),
	}

	// Query the remote World graph with the encoded filter.
	resp, err := ws.service.LookupGraphQuads(ctx, &s4wave_world.LookupGraphQuadsRequest{
		Filter: protoFilter,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}

	// Expose the returned quads through the World graph interface.
	quads := make([]world.GraphQuad, len(resp.Quads))
	for i, q := range resp.Quads {
		quads[i] = q
	}
	return quads, nil
}

// LookupGraphQuadsBatch searches for graph quads using bounded indexed filters.
func (ws *SDKWorldState) LookupGraphQuadsBatch(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	// Encode each graph filter for the batch resource request.
	protoFilters := make([]*quad.Quad, len(filters))
	for i, filter := range filters {
		protoFilters[i] = &quad.Quad{
			Subject:   filter.GetSubject(),
			Predicate: filter.GetPredicate(),
			Obj:       filter.GetObj(),
			Label:     filter.GetLabel(),
		}
	}

	// Query the remote World graph with the bounded batch of filters.
	resp, err := ws.service.LookupGraphQuadsBatch(ctx, &s4wave_world.LookupGraphQuadsBatchRequest{
		Filters:        protoFilters,
		LimitPerFilter: limitPerFilter,
	})
	if err != nil {
		return nil, err
	}

	// Expose each result batch through the World graph interface.
	results := make([][]world.GraphQuad, len(resp.GetResults()))
	for i, result := range resp.GetResults() {
		// Convert the quads returned for this graph filter.
		quads := make([]world.GraphQuad, len(result.GetQuads()))
		for j, q := range result.GetQuads() {
			quads[j] = q
		}

		// Preserve the graph filter order in the batch results.
		results[i] = quads
	}
	return results, nil
}

// ListGraphEdgeBuckets lists grouped inbound/outbound graph edge buckets.
func (ws *SDKWorldState) ListGraphEdgeBuckets(ctx context.Context, query *world.GraphEdgeBucketQuery) ([]*world.GraphEdgeBucket, error) {
	// Request grouped edge buckets from the remote World graph.
	req := s4wave_world.GraphEdgeBucketQueryToProto(query)
	resp, err := ws.service.ListGraphEdgeBuckets(ctx, req)
	if err != nil {
		return nil, err
	}

	// Convert the grouped edge buckets to World graph records.
	buckets := make([]*world.GraphEdgeBucket, len(resp.GetBuckets()))
	for i, bucket := range resp.GetBuckets() {
		// Expose the outgoing bucket edges through the World graph interface.
		outgoing := make([]world.GraphQuad, len(bucket.GetOutgoing()))
		for j, q := range bucket.GetOutgoing() {
			outgoing[j] = q
		}

		// Expose the incoming bucket edges through the World graph interface.
		incoming := make([]world.GraphQuad, len(bucket.GetIncoming()))
		for j, q := range bucket.GetIncoming() {
			incoming[j] = q
		}

		// Retain the bucket origin and truncation flags with its converted edges.
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
	// Request root references for the specified remote World objects.
	resp, err := ws.service.GetObjectRootRefsBatch(ctx, &s4wave_world.GetObjectRootRefsBatchRequest{
		ObjectKeys: keys,
	})
	if err != nil {
		return nil, err
	}

	// Copy the remote object roots and revisions into World records.
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

	return objectMetadataFromProto(resp.GetMetadata()), nil
}

// objectMetadataFromProto decodes object metadata from the World resource.
func objectMetadataFromProto(in []*s4wave_world.ObjectMetadata) []*world_types.ObjectMetadata {
	metadata := make([]*world_types.ObjectMetadata, len(in))
	for i, md := range in {
		metadata[i] = &world_types.ObjectMetadata{
			ObjectKey:       md.GetObjectKey(),
			TypeID:          md.GetTypeId(),
			ParentObjectKey: md.GetParentObjectKey(),
		}
	}
	return metadata
}

// ForEachObjectBodyPage calls cb with each page of serialized object bodies.
// The callback must finish with the page before the next RPC response is read.
func (ws *SDKWorldState) ForEachObjectBodyPage(
	ctx context.Context,
	keys []string,
	cb func([]*world.ObjectBody) error,
) error {
	return s4wave_world.ForEachObjectBodyPage(ctx, ws.service, keys, cb)
}

// GetObjectBodiesBatch returns serialized object bodies for object keys.
func (ws *SDKWorldState) GetObjectBodiesBatch(ctx context.Context, keys []string) ([]*world.ObjectBody, error) {
	return s4wave_world.GetObjectBodiesBatch(ctx, ws.service, keys)
}

// QueryGraphPath executes a bounded server-side graph path query.
func (ws *SDKWorldState) QueryGraphPath(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	// Encode the bounded graph path query for the resource service.
	req, err := s4wave_world.GraphPathQueryToProto(query)
	if err != nil {
		return nil, err
	}

	// Open the remote graph path query resource.
	resp, err := ws.service.QueryGraphPath(ctx, req)
	if err != nil {
		return nil, err
	}

	// Acquire the graph query client and release its resource when collection ends.
	ref := ws.client.CreateResourceReference(resp.GetResourceId())
	defer ref.Release()
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	service := s4wave_world.NewSRPCGraphPathQueryResourceServiceClient(srpcClient)
	defer service.Close(ctx, &s4wave_world.CloseGraphPathQueryRequest{})

	// Collect graph path pages until the remote query is complete.
	result := &world.GraphPathQueryResult{}
	for {
		// Read the next page from the graph query resource.
		page, err := service.Next(ctx, &s4wave_world.NextGraphPathQueryRequest{})
		if err != nil {
			return nil, err
		}

		// Accumulate the page object keys and graph quads.
		result.ObjectKeys = append(result.ObjectKeys, page.GetObjectKeys()...)
		for _, q := range page.GetQuads() {
			result.Quads = append(result.Quads, q)
		}

		// Return the collected graph path once the final page arrives.
		if page.GetDone() {
			return result, nil
		}
	}
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
func (ws *SDKWorldState) DeleteGraphObject(ctx context.Context, value string) error {
	// Preserve the read-only error identity before contacting the server.
	if ws.GetReadOnly() {
		return tx.ErrNotWrite
	}

	// Delete the remote object's graph quads.
	_, err := ws.service.DeleteGraphObject(ctx, &s4wave_world.DeleteGraphObjectRequest{ObjectKey: value})
	return err
}

// ApplyWorldOp applies a batch operation at the world level.
// The handling of the operation is operation-type specific.
// Returns seqno, sysErr, err.
func (ws *SDKWorldState) ApplyWorldOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	// Preserve the read-only error identity before encoding or sending the operation.
	if ws.GetReadOnly() {
		return 0, false, tx.ErrNotWrite
	}

	// Encode the World operation for the resource service.
	opData, err := op.MarshalBlock()
	if err != nil {
		return 0, false, err
	}

	// Apply the encoded operation to the remote World and return its outcome.
	resp, err := ws.service.ApplyWorldOp(ctx, &s4wave_world.ApplyWorldOpRequest{
		OpTypeId: op.GetOperationTypeId(),
		OpData:   opData,
		OpSender: sender.String(),
	})
	if err != nil {
		return 0, false, err
	}
	if err := resp.GetError(); err != nil {
		return 0, false, err
	}
	return resp.Seqno, resp.SysErr, nil
}

// HasObject reports whether an object exists at key. The client keeps no
// transaction-local knowledge, so it queries the remote resource directly.
func (ws *SDKWorldState) HasObject(ctx context.Context, key string) (bool, error) {
	obj, found, err := ws.GetObject(ctx, key)
	world.ReleaseObjectState(obj)
	return found, err
}

// _ is a type assertion.
var _ world.WorldState = (*SDKWorldState)(nil)
