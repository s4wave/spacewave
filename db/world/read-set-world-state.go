package world

import (
	"context"
	"sync/atomic"

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

// BuildStorageCursor records nothing. The cursor starts at an empty ref, so it
// reads only blocks named by refs the caller holds, and blocks never change.
func (s *readSetWorldState) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return s.WorldState.BuildStorageCursor(ctx)
}

// AccessWorldState records a read of the whole World when ref is empty, since
// the cursor then points at the root World state. A ref names blocks that
// never change, so following it records nothing.
func (s *readSetWorldState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	if ref.GetEmpty() {
		s.reads.markAll()
	}
	return s.WorldState.AccessWorldState(ctx, ref, cb)
}

// StageWorldState records a raw storage access.
func (s *readSetWorldState) StageWorldState(ctx context.Context) (WorldStage, error) {
	s.reads.markAll()
	return s.WorldState.StageWorldState(ctx)
}

// ApplyWorldOp records the reads the operation makes. The World state that runs
// it passes its handle through TrackOperationReads; when none does, the
// operation may have read anything, and the whole World is read.
func (s *readSetWorldState) ApplyWorldOp(
	ctx context.Context,
	op Operation,
	opSender peer.ID,
) (uint64, bool, error) {
	// Carry the reads to the World state that runs the operation.
	carried := &operationReads{reads: s.reads}
	opCtx := context.WithValue(ctx, operationReadsKey{}, carried)
	seqno, sysErr, err := s.WorldState.ApplyWorldOp(opCtx, op, opSender)

	// An operation run without tracking hides its reads.
	if !carried.tracked.Load() {
		s.reads.markAll()
	}
	return seqno, sysErr, err
}

// operationReads carries a ReadSet to the World state that runs an operation.
type operationReads struct {
	// reads records the operation's reads.
	reads *ReadSet
	// tracked is set once a World state ran the operation through reads.
	tracked atomic.Bool
}

// operationReadsKey identifies the operationReads a context carries.
type operationReadsKey struct{}

// TrackOperationReads returns the handle to run an operation against: ws
// recording its reads in the ReadSet ctx carries from a tracked ApplyWorldOp,
// or ws itself when ctx carries none. A World state that calls an Operation's
// ApplyWorldOp passes its own handle through it, so a write's read set holds
// the operation's reads rather than the whole World.
func TrackOperationReads(ctx context.Context, ws WorldState) WorldState {
	carried, _ := ctx.Value(operationReadsKey{}).(*operationReads)
	if carried == nil {
		return ws
	}
	carried.tracked.Store(true)
	return carried.reads.Track(ws)
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

// PurgeObject records the object key and purges it through the wrapped state.
func (s *readSetWorldState) PurgeObject(ctx context.Context, key string) (bool, error) {
	s.reads.addKeys(key)
	s.reads.markGraph()
	return PurgeObject(ctx, s.WorldState, key)
}

// GetObjectRootRefsBatch records the requested keys and reads their roots
// through the wrapped state's batcher when it has one.
func (s *readSetWorldState) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*ObjectRootRef, error) {
	s.reads.addKeys(keys...)
	return GetObjectRootRefsBatch(ctx, s.WorldState, keys)
}

// GetObjectBodiesBatch records the requested keys and reads their bodies
// through the wrapped state's batcher when it has one.
func (s *readSetWorldState) GetObjectBodiesBatch(ctx context.Context, keys []string) ([]*ObjectBody, error) {
	s.reads.addKeys(keys...)
	return GetObjectBodiesBatch(ctx, s.WorldState, keys)
}

// GetObjectBodiesBatchPage records the requested keys and pages their bodies
// through the wrapped state.
func (s *readSetWorldState) GetObjectBodiesBatchPage(ctx context.Context, keys []string, byteBudget int) ([]*ObjectBody, uint32, error) {
	s.reads.addKeys(keys...)
	return GetObjectBodiesBatchPage(ctx, s.WorldState, keys, byteBudget)
}

// GetObjectBodiesBatchPageWithSeqno records the requested keys and pages their
// bodies with the wrapped state's seqno.
func (s *readSetWorldState) GetObjectBodiesBatchPageWithSeqno(
	ctx context.Context,
	keys []string,
	byteBudget int,
) ([]*ObjectBody, uint32, uint64, error) {
	// Page through the wrapped state's batcher when it has one.
	s.reads.addKeys(keys...)
	if pager, ok := s.WorldState.(ObjectBodyPageSeqnoBatcher); ok {
		return pager.GetObjectBodiesBatchPageWithSeqno(ctx, keys, byteBudget)
	}

	// Read the seqno, then the page.
	seqno, err := s.GetSeqno(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	bodies, consumed, err := GetObjectBodiesBatchPage(ctx, s.WorldState, keys, byteBudget)
	return bodies, consumed, seqno, err
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

// ListObjectsWithType records the type edges it lists and forwards its page.
func (s *readSetWorldState) ListObjectsWithType(ctx context.Context, typeID, startAfter string, limit uint32) ([]string, bool, error) {
	s.reads.addPatterns(NewTypeQuadFilter(typeID))
	return s.WorldState.ListObjectsWithType(ctx, typeID, startAfter, limit)
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
var (
	_ WorldState                 = (*readSetWorldState)(nil)
	_ ObjectRootRefBatcher       = (*readSetWorldState)(nil)
	_ ObjectBodyBatcher          = (*readSetWorldState)(nil)
	_ ObjectBodyPageBatcher      = (*readSetWorldState)(nil)
	_ ObjectBodyPageSeqnoBatcher = (*readSetWorldState)(nil)
)
