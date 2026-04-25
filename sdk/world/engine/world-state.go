package sdk_world_engine

import (
	"context"

	"github.com/aperturerobotics/cayley/graph/memstore"
	cayley_quad "github.com/aperturerobotics/cayley/quad"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// SDKWorldState implements world.WorldState over SRPC by delegating to
// WorldStateResourceService calls on a remote resource.
type SDKWorldState struct {
	client   *resource_client.Client
	ref      resource_client.ResourceRef
	service  s4wave_world.SRPCWorldStateResourceServiceClient
	readOnly bool
}

// NewSDKWorldState creates a new SDKWorldState wrapping a resource reference.
func NewSDKWorldState(client *resource_client.Client, ref resource_client.ResourceRef, readOnly bool) (*SDKWorldState, error) {
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

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
func (ws *SDKWorldState) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad("", "", "", ""), 0)
	if err != nil {
		return err
	}
	cquads := make([]cayley_quad.Quad, 0, len(quads))
	for _, q := range quads {
		cq, err := world.GraphQuadToCayleyQuad(q, false)
		if err != nil {
			return err
		}
		cquads = append(cquads, cq)
	}
	return cb(ctx, memstore.New(cquads...))
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
