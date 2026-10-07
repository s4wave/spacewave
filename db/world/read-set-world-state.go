package world

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/net/peer"
)

// readSetWorldState records the reads made through a WorldState in a ReadSet.
// Writes record the keys and quads they touch, since a write may read them.
type readSetWorldState struct {
	WorldState
	reads *ReadSet
}

// BuildStorageCursor records a raw storage read.
func (s *readSetWorldState) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	s.reads.markAll()
	return s.WorldState.BuildStorageCursor(ctx)
}

// AccessWorldState records a raw storage read.
func (s *readSetWorldState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	s.reads.markAll()
	return s.WorldState.AccessWorldState(ctx, ref, cb)
}

// StageWorldState records a raw storage access.
func (s *readSetWorldState) StageWorldState(ctx context.Context) (WorldStage, error) {
	s.reads.markAll()
	return s.WorldState.StageWorldState(ctx)
}

// ApplyWorldOp records an operation, whose reads are not visible here.
func (s *readSetWorldState) ApplyWorldOp(
	ctx context.Context,
	op Operation,
	opSender peer.ID,
) (uint64, bool, error) {
	s.reads.markAll()
	return s.WorldState.ApplyWorldOp(ctx, op, opSender)
}

// GetObject records the object key.
func (s *readSetWorldState) GetObject(ctx context.Context, key string) (ObjectState, bool, error) {
	s.reads.addKeys(key)
	return s.WorldState.GetObject(ctx, key)
}

// IterateObjects records the key prefix.
func (s *readSetWorldState) IterateObjects(ctx context.Context, prefix string, reversed bool) ObjectIterator {
	s.reads.addPrefix(prefix)
	return s.WorldState.IterateObjects(ctx, prefix, reversed)
}

// CreateObject records the object key.
func (s *readSetWorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (ObjectState, error) {
	s.reads.addKeys(key)
	return s.WorldState.CreateObject(ctx, key, rootRef)
}

// RenameObject records both keys, and both prefixes when renaming descendants.
func (s *readSetWorldState) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (ObjectState, error) {
	if descendants {
		s.reads.addPrefix(oldKey)
		s.reads.addPrefix(newKey)
	}
	s.reads.addKeys(oldKey, newKey)
	return s.WorldState.RenameObject(ctx, oldKey, newKey, descendants)
}

// DeleteObject records the object key.
func (s *readSetWorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	s.reads.addKeys(key)
	return s.WorldState.DeleteObject(ctx, key)
}

// HasObject records the object key.
func (s *readSetWorldState) HasObject(ctx context.Context, key string) (bool, error) {
	s.reads.addKeys(key)
	return s.WorldState.HasObject(ctx, key)
}

// AccessCayleyGraph records a graph read that patterns cannot describe.
func (s *readSetWorldState) AccessCayleyGraph(
	ctx context.Context,
	write bool,
	cb func(ctx context.Context, h CayleyHandle) error,
) error {
	s.reads.markGraph()
	return s.WorldState.AccessCayleyGraph(ctx, write, cb)
}

// LookupGraphQuads records the filter.
func (s *readSetWorldState) LookupGraphQuads(ctx context.Context, filter GraphQuad, limit uint32) ([]GraphQuad, error) {
	s.reads.addPatterns(filter)
	return s.WorldState.LookupGraphQuads(ctx, filter, limit)
}

// LookupGraphQuadsBatch records the filters.
func (s *readSetWorldState) LookupGraphQuadsBatch(ctx context.Context, filters []GraphQuad, limitPerFilter uint32) ([][]GraphQuad, error) {
	s.reads.addPatterns(filters...)
	return s.WorldState.LookupGraphQuadsBatch(ctx, filters, limitPerFilter)
}

// QueryGraphPath records a graph read that patterns cannot describe.
func (s *readSetWorldState) QueryGraphPath(ctx context.Context, query *GraphPathQuery) (*GraphPathQueryResult, error) {
	s.reads.markGraph()
	return s.WorldState.QueryGraphPath(ctx, query)
}

// SetGraphQuad records the quad.
func (s *readSetWorldState) SetGraphQuad(ctx context.Context, q GraphQuad) error {
	s.reads.addPatterns(q)
	return s.WorldState.SetGraphQuad(ctx, q)
}

// DeleteGraphQuad records the quad.
func (s *readSetWorldState) DeleteGraphQuad(ctx context.Context, q GraphQuad) error {
	s.reads.addPatterns(q)
	return s.WorldState.DeleteGraphQuad(ctx, q)
}

// DeleteGraphObject records a graph write that patterns cannot describe.
func (s *readSetWorldState) DeleteGraphObject(ctx context.Context, value string) error {
	s.reads.markGraph()
	return s.WorldState.DeleteGraphObject(ctx, value)
}

// _ is a type assertion
var _ WorldState = (*readSetWorldState)(nil)
