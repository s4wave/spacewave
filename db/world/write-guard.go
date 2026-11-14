package world

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/net/peer"
)

// WriteGuard decides which writes a guarded WorldState may make. Every write
// through a state GuardWorldState returns, through the object states it
// returns, and through the block stores of the cursors and stages they open,
// is described to CheckWrite first and is not made when CheckWrite returns an
// error.
//
// A guarded cursor puts blocks freely: a block is content addressed and
// changes no World object until a guarded write names it, such as SetRootRef
// or an op. Removing a block, moving a named root and recording a completion
// proof through a guarded cursor are unscoped writes.
//
// The guarded types spell out every method of the interfaces they implement,
// so a method added to an interface fails to compile until it is guarded.
type WriteGuard interface {
	// CheckWrite returns nil when w may be made on ws, or the refusal. ws is
	// the unguarded state the write would change, for the guard's reads. A
	// refusal is an *OperationRejection; any other error is a failure to
	// decide, such as a read error, and is not final.
	CheckWrite(ctx context.Context, ws WorldState, w *GuardedWrite) error
}

// GuardedWrite describes one write to a WriteGuard.
type GuardedWrite struct {
	// OpTypeID is the type of the World or object op the write applies, or
	// empty for a direct write.
	OpTypeID string
	// ObjectKeys are the objects the write creates, changes or deletes.
	ObjectKeys []string
	// Quad is the graph quad the write sets or deletes, or nil.
	Quad GraphQuad
	// Unscoped marks a write that can change objects ObjectKeys does not
	// name: a World op that is not a WriteKeysOperation, a writable graph
	// handle, a rename of an object's descendants, and a block removal, root
	// move or completion proof through a guarded cursor.
	Unscoped bool
}

// IsGuardSysErr reports whether err, returned by a WriteGuard, is a system
// error rather than a refusal: only an *OperationRejection is final.
func IsGuardSysErr(err error) bool {
	var rejection *OperationRejection
	return err != nil && !errors.As(err, &rejection)
}

// WorldStateGetter is an ObjectState that returns the WorldState holding it,
// so a component that receives only the object can guard writes through it.
type WorldStateGetter interface {
	// GetWorldState returns the unguarded WorldState that holds the object.
	GetWorldState() WorldState
}

// GuardWorldState returns ws with every write checked by guard first.
// A nil guard returns ws.
func GuardWorldState(ws WorldState, guard WriteGuard) WorldState {
	if guard == nil || ws == nil {
		return ws
	}
	switch g := ws.(type) {
	case *guardedWorldState:
		if g.guard == guard {
			return ws
		}
	case *guardedTx:
		if g.guard == guard {
			return ws
		}
	}
	return &guardedWorldState{ws: ws, guard: guard}
}

// GuardObjectState returns obj, held by the unguarded ws, with every write
// checked by guard first. A nil guard returns obj.
func GuardObjectState(obj ObjectState, ws WorldState, guard WriteGuard) ObjectState {
	if guard == nil || obj == nil {
		return obj
	}
	if g, ok := obj.(*guardedObjectState); ok && g.guard == guard {
		return obj
	}
	return &guardedObjectState{obj: obj, ws: ws, guard: guard}
}

// GuardEngine returns e with every transaction it opens guarded as
// GuardWorldState guards a state, so a component handed the engine cannot
// write past guard. A nil guard returns e. The guarded engine does not expose
// optional engine interfaces, such as root retention, that write past a
// transaction. It rehearses when e does, with the guard checking the
// rehearsal's writes too, so a refused write ends the rehearsal before the
// locked apply takes the writer.
func GuardEngine(e Engine, guard WriteGuard) Engine {
	// Keep an engine guard already guards.
	if guard == nil || e == nil {
		return e
	}
	switch g := e.(type) {
	case *guardedEngine:
		if g.guard == guard {
			return e
		}
	case *guardedRehearsalEngine:
		if g.guard == guard {
			return e
		}
	}

	// Guard it, rehearsing when it rehearses.
	ge := &guardedEngine{e: e, guard: guard}
	if re, ok := e.(RehearsalEngine); ok {
		return &guardedRehearsalEngine{guardedEngine: ge, re: re}
	}
	return ge
}

// unscopedCheck returns the check a guarded cursor makes before an unscoped
// write to the store of ws.
func unscopedCheck(ws WorldState, guard WriteGuard) func(context.Context) error {
	return func(ctx context.Context) error {
		return guard.CheckWrite(ctx, ws, &GuardedWrite{Unscoped: true})
	}
}

// guardedAccess calls cb with cursors access opens, guarded by check.
func guardedAccess(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
	check func(context.Context) error,
	access func(context.Context, *bucket.ObjectRef, func(*bucket_lookup.Cursor) error) error,
) error {
	return access(ctx, ref, func(cursor *bucket_lookup.Cursor) error {
		if err := guardCursor(ctx, cursor, check); err != nil {
			return err
		}
		return cb(cursor)
	})
}

// guardedStorageCursor returns the cursor build opens, guarded by check.
func guardedStorageCursor(
	ctx context.Context,
	check func(context.Context) error,
	build func(context.Context) (*bucket_lookup.Cursor, error),
) (*bucket_lookup.Cursor, error) {
	cursor, err := build(ctx)
	if err != nil {
		return nil, err
	}
	if err := guardCursor(ctx, cursor, check); err != nil {
		cursor.Release()
		return nil, err
	}
	return cursor, nil
}

// guardedEngine guards each transaction and cursor the wrapped engine opens.
type guardedEngine struct {
	// e is the wrapped engine.
	e Engine
	// guard checks each write.
	guard WriteGuard
}

// check returns the unscoped check for the engine's cursors, which read the
// engine's newest World.
func (e *guardedEngine) check() func(context.Context) error {
	return unscopedCheck(NewEngineWorldState(e.e, false), e.guard)
}

// OperationAuthor returns the wrapped engine's author.
func (e *guardedEngine) OperationAuthor(ctx context.Context) (peer.ID, string, error) {
	return e.e.OperationAuthor(ctx)
}

// NewTransaction opens a transaction whose writes the guard checks.
func (e *guardedEngine) NewTransaction(ctx context.Context, write bool) (Tx, error) {
	t, err := e.e.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return e.guardTx(t), nil
}

// guardTx returns t with its writes checked by the guard.
func (e *guardedEngine) guardTx(t Tx) Tx {
	return &guardedTx{guardedWorldState: &guardedWorldState{ws: t, guard: e.guard}, tx: t}
}

// Sync fences the wrapped engine's blocks and head; it writes no object.
func (e *guardedEngine) Sync(ctx context.Context) (bool, error) {
	return e.e.Sync(ctx)
}

// SyncThrough waits for the wrapped engine's durable head.
func (e *guardedEngine) SyncThrough(ctx context.Context, seqno uint64) (uint64, error) {
	return e.e.SyncThrough(ctx, seqno)
}

// BuildStorageCursor builds a guarded cursor on the engine's storage.
func (e *guardedEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return guardedStorageCursor(ctx, e.check(), e.e.BuildStorageCursor)
}

// AccessWorldState calls cb with a guarded cursor on the engine's storage.
func (e *guardedEngine) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return guardedAccess(ctx, ref, cb, e.check(), e.e.AccessWorldState)
}

// StageWorldState opens a guarded stage on the engine.
func (e *guardedEngine) StageWorldState(ctx context.Context) (WorldStage, error) {
	stage, err := e.e.StageWorldState(ctx)
	return guardStage(stage, err, e.check())
}

// GetSeqno returns the wrapped engine's seqno.
func (e *guardedEngine) GetSeqno(ctx context.Context) (uint64, error) {
	return e.e.GetSeqno(ctx)
}

// WaitSeqno waits on the wrapped engine.
func (e *guardedEngine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return e.e.WaitSeqno(ctx, value)
}

// WatchChanges watches the wrapped engine.
func (e *guardedEngine) WatchChanges(ctx context.Context, afterSeqno uint64, touches ChangeFilter, cb func(seqno uint64, changes *ChangeSet) error) error {
	return e.e.WatchChanges(ctx, afterSeqno, touches, cb)
}

// WatchIndexedChanges watches through the wrapped engine.
func (e *guardedEngine) WatchIndexedChanges(ctx context.Context, afterSeqno uint64, interest *ChangeInterest, cb func(uint64, *ChangeSet) error) error {
	return WatchInterestedChanges(ctx, e.e, afterSeqno, interest, cb)
}

// WaitObjectRev waits on the wrapped engine.
func (e *guardedEngine) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	return e.e.WaitObjectRev(ctx, key, rev, ignoreNotFound)
}

// guardedRehearsalEngine is a guarded engine whose wrapped engine rehearses.
type guardedRehearsalEngine struct {
	*guardedEngine
	re RehearsalEngine
}

// NewRehearsalTransaction opens a rehearsal whose writes the guard checks.
func (e *guardedRehearsalEngine) NewRehearsalTransaction(ctx context.Context) (Tx, error) {
	t, err := e.re.NewRehearsalTransaction(ctx)
	if err != nil {
		return nil, err
	}
	return e.guardTx(t), nil
}

// guardedTx is a transaction whose writes the guard checks.
type guardedTx struct {
	*guardedWorldState
	// tx supplies Commit and Discard.
	tx Tx
}

// Commit commits the wrapped transaction.
func (t *guardedTx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

// Discard discards the wrapped transaction.
func (t *guardedTx) Discard() {
	t.tx.Discard()
}

// guardStage returns stage with its cursors guarded by check, or err.
func guardStage(stage WorldStage, err error, check func(context.Context) error) (WorldStage, error) {
	if err != nil || stage == nil {
		return stage, err
	}
	return &guardedStage{stage: stage, check: check}, nil
}

// guardedStage is a stage whose cursors are guarded. A stage only builds
// blocks: what it builds reaches the World only through a guarded write that
// names it, such as SetRootRef or an op on the guarded state.
type guardedStage struct {
	// stage is the wrapped stage.
	stage WorldStage
	// check describes an unscoped write to the guard.
	check func(context.Context) error
}

// BuildStorageCursor builds a guarded cursor on the stage.
func (s *guardedStage) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return guardedStorageCursor(ctx, s.check, s.stage.BuildStorageCursor)
}

// AccessWorldState calls cb with a guarded cursor on the stage.
func (s *guardedStage) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return guardedAccess(ctx, ref, cb, s.check, s.stage.AccessWorldState)
}

// ReleaseRoots drops the stage's own holds on roots built through it.
func (s *guardedStage) ReleaseRoots(ctx context.Context, roots []*block.BlockRef) error {
	return s.stage.ReleaseRoots(ctx, roots)
}

// Release ends the wrapped stage.
func (s *guardedStage) Release() {
	s.stage.Release()
}

// guardedWorldState checks each write with its guard before making it, and
// forwards every read and optional read interface of the wrapped state.
type guardedWorldState struct {
	// ws is the wrapped, unguarded state.
	ws WorldState
	// guard checks each write.
	guard WriteGuard
}

// check describes w to the guard.
func (s *guardedWorldState) check(ctx context.Context, w *GuardedWrite) error {
	return s.guard.CheckWrite(ctx, s.ws, w)
}

// object wraps an object state returned by the wrapped state.
func (s *guardedWorldState) object(obj ObjectState, err error) (ObjectState, error) {
	if obj == nil {
		return obj, err
	}
	return &guardedObjectState{obj: obj, ws: s.ws, guard: s.guard}, err
}

// GetReadOnly returns whether the wrapped state is read-only.
func (s *guardedWorldState) GetReadOnly() bool {
	return s.ws.GetReadOnly()
}

// Sync fences the wrapped state's blocks; it writes no object.
func (s *guardedWorldState) Sync(ctx context.Context) (bool, error) {
	return s.ws.Sync(ctx)
}

// ListObjectsWithType lists through the wrapped state.
func (s *guardedWorldState) ListObjectsWithType(ctx context.Context, typeID, startAfter string, limit uint32) ([]string, bool, error) {
	return s.ws.ListObjectsWithType(ctx, typeID, startAfter, limit)
}

// BuildStorageCursor builds a guarded cursor on the wrapped state's storage.
func (s *guardedWorldState) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	return guardedStorageCursor(ctx, unscopedCheck(s.ws, s.guard), s.ws.BuildStorageCursor)
}

// AccessWorldState calls cb with a guarded cursor on the wrapped state's
// storage.
func (s *guardedWorldState) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return guardedAccess(ctx, ref, cb, unscopedCheck(s.ws, s.guard), s.ws.AccessWorldState)
}

// StageWorldState opens a guarded stage on the wrapped state.
func (s *guardedWorldState) StageWorldState(ctx context.Context) (WorldStage, error) {
	stage, err := s.ws.StageWorldState(ctx)
	return guardStage(stage, err, unscopedCheck(s.ws, s.guard))
}

// GetObject returns a guarded object state.
func (s *guardedWorldState) GetObject(ctx context.Context, key string) (ObjectState, bool, error) {
	obj, found, err := s.ws.GetObject(ctx, key)
	obj, err = s.object(obj, err)
	return obj, found, err
}

// IterateObjects iterates keys through the wrapped state; it yields no
// object state.
func (s *guardedWorldState) IterateObjects(ctx context.Context, prefix string, reversed bool) ObjectIterator {
	return s.ws.IterateObjects(ctx, prefix, reversed)
}

// CreateObject checks the new key.
func (s *guardedWorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (ObjectState, error) {
	if err := s.check(ctx, &GuardedWrite{ObjectKeys: []string{key}}); err != nil {
		return nil, err
	}
	return s.object(s.ws.CreateObject(ctx, key, rootRef))
}

// RenameObject checks both keys. Renaming descendants is unscoped.
func (s *guardedWorldState) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (ObjectState, error) {
	w := &GuardedWrite{ObjectKeys: []string{oldKey, newKey}, Unscoped: descendants}
	if err := s.check(ctx, w); err != nil {
		return nil, err
	}
	return s.object(s.ws.RenameObject(ctx, oldKey, newKey, descendants))
}

// DeleteObject checks the key.
func (s *guardedWorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	if err := s.check(ctx, &GuardedWrite{ObjectKeys: []string{key}}); err != nil {
		return false, err
	}
	return s.ws.DeleteObject(ctx, key)
}

// HasObject checks through the wrapped state.
func (s *guardedWorldState) HasObject(ctx context.Context, key string) (bool, error) {
	return s.ws.HasObject(ctx, key)
}

// PurgeObject checks the key and purges through the wrapped state.
func (s *guardedWorldState) PurgeObject(ctx context.Context, key string) (bool, error) {
	if err := s.check(ctx, &GuardedWrite{ObjectKeys: []string{key}}); err != nil {
		return false, err
	}
	return PurgeObject(ctx, s.ws, key)
}

// AccessCayleyGraph refuses a writable handle unless the guard allows an
// unscoped write.
func (s *guardedWorldState) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h CayleyHandle) error) error {
	if write {
		if err := s.check(ctx, &GuardedWrite{Unscoped: true}); err != nil {
			return err
		}
	}
	return s.ws.AccessCayleyGraph(ctx, write, cb)
}

// LookupGraphQuads reads through the wrapped state.
func (s *guardedWorldState) LookupGraphQuads(ctx context.Context, filter GraphQuad, limit uint32) ([]GraphQuad, error) {
	return s.ws.LookupGraphQuads(ctx, filter, limit)
}

// LookupGraphQuadsBatch reads through the wrapped state.
func (s *guardedWorldState) LookupGraphQuadsBatch(ctx context.Context, filters []GraphQuad, limitPerFilter uint32) ([][]GraphQuad, error) {
	return s.ws.LookupGraphQuadsBatch(ctx, filters, limitPerFilter)
}

// QueryGraphPath reads through the wrapped state.
func (s *guardedWorldState) QueryGraphPath(ctx context.Context, query *GraphPathQuery) (*GraphPathQueryResult, error) {
	return s.ws.QueryGraphPath(ctx, query)
}

// SetGraphQuad checks the quad.
func (s *guardedWorldState) SetGraphQuad(ctx context.Context, q GraphQuad) error {
	if err := s.check(ctx, &GuardedWrite{Quad: q}); err != nil {
		return err
	}
	return s.ws.SetGraphQuad(ctx, q)
}

// DeleteGraphQuad checks the quad.
func (s *guardedWorldState) DeleteGraphQuad(ctx context.Context, q GraphQuad) error {
	if err := s.check(ctx, &GuardedWrite{Quad: q}); err != nil {
		return err
	}
	return s.ws.DeleteGraphQuad(ctx, q)
}

// DeleteGraphObject checks the object whose quads it deletes.
func (s *guardedWorldState) DeleteGraphObject(ctx context.Context, value string) error {
	if err := s.check(ctx, &GuardedWrite{ObjectKeys: []string{value}}); err != nil {
		return err
	}
	return s.ws.DeleteGraphObject(ctx, value)
}

// ApplyWorldOp checks the op's type and the keys it names. An op that does not
// name its keys is an unscoped write. A guard that fails to decide returns a
// system error, so the op is retried rather than rejected.
func (s *guardedWorldState) ApplyWorldOp(ctx context.Context, op Operation, opSender peer.ID) (uint64, bool, error) {
	// Describe the op by its type and the keys it names.
	if op == nil {
		return 0, false, ErrEmptyOp
	}
	w := &GuardedWrite{OpTypeID: op.TypeID()}
	if keyed, ok := op.(WriteKeysOperation); ok {
		w.ObjectKeys = keyed.WriteObjectKeys()
	} else {
		w.Unscoped = true
	}

	// Apply the op once the guard allows it.
	if err := s.check(ctx, w); err != nil {
		return 0, IsGuardSysErr(err), err
	}
	return s.ws.ApplyWorldOp(ctx, op, opSender)
}

// GetSeqno returns the wrapped state's seqno.
func (s *guardedWorldState) GetSeqno(ctx context.Context) (uint64, error) {
	return s.ws.GetSeqno(ctx)
}

// WaitSeqno waits on the wrapped state.
func (s *guardedWorldState) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return s.ws.WaitSeqno(ctx, value)
}

// GetObjectRootRefsBatch reads through the wrapped state's batcher.
func (s *guardedWorldState) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*ObjectRootRef, error) {
	return GetObjectRootRefsBatch(ctx, s.ws, keys)
}

// GetObjectBodiesBatch reads through the wrapped state's batcher.
func (s *guardedWorldState) GetObjectBodiesBatch(ctx context.Context, keys []string) ([]*ObjectBody, error) {
	return GetObjectBodiesBatch(ctx, s.ws, keys)
}

// GetObjectBodiesBatchPage reads through the wrapped state's batcher.
func (s *guardedWorldState) GetObjectBodiesBatchPage(ctx context.Context, keys []string, byteBudget int) ([]*ObjectBody, uint32, error) {
	return GetObjectBodiesBatchPage(ctx, s.ws, keys, byteBudget)
}

// GetObjectBodiesBatchPageWithSeqno reads through the wrapped state's batcher,
// or reads the seqno and then the page.
func (s *guardedWorldState) GetObjectBodiesBatchPageWithSeqno(ctx context.Context, keys []string, byteBudget int) ([]*ObjectBody, uint32, uint64, error) {
	// Read through the wrapped pager when it has one.
	if pager, ok := s.ws.(ObjectBodyPageSeqnoBatcher); ok {
		return pager.GetObjectBodiesBatchPageWithSeqno(ctx, keys, byteBudget)
	}

	// Otherwise read the seqno, then the page.
	seqno, err := s.ws.GetSeqno(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	bodies, consumed, err := GetObjectBodiesBatchPage(ctx, s.ws, keys, byteBudget)
	return bodies, consumed, seqno, err
}

// WaitObjectRev waits through the wrapped state.
func (s *guardedWorldState) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	return WaitObjectRev(ctx, s.ws, key, rev, ignoreNotFound)
}

// WatchChanges watches through the wrapped state.
func (s *guardedWorldState) WatchChanges(ctx context.Context, afterSeqno uint64, touches ChangeFilter, cb func(seqno uint64, changes *ChangeSet) error) error {
	return WatchTouchingChanges(ctx, s.ws, afterSeqno, touches, cb)
}

// WatchIndexedChanges watches through the wrapped state.
func (s *guardedWorldState) WatchIndexedChanges(ctx context.Context, afterSeqno uint64, interest *ChangeInterest, cb func(seqno uint64, changes *ChangeSet) error) error {
	return WatchInterestedChanges(ctx, s.ws, afterSeqno, interest, cb)
}

// guardedObjectState checks each write to one object with its guard.
type guardedObjectState struct {
	// obj is the wrapped, unguarded object state.
	obj ObjectState
	// ws is the unguarded state that holds the object, for the guard's reads.
	ws WorldState
	// guard checks each write.
	guard WriteGuard
}

// check describes a write to this object to the guard.
func (o *guardedObjectState) check(ctx context.Context, opTypeID string) error {
	w := &GuardedWrite{OpTypeID: opTypeID, ObjectKeys: []string{o.obj.GetKey()}}
	return o.guard.CheckWrite(ctx, o.ws, w)
}

// GetKey returns the object's key.
func (o *guardedObjectState) GetKey() string {
	return o.obj.GetKey()
}

// GetRootRef reads the object's root.
func (o *guardedObjectState) GetRootRef(ctx context.Context) (*bucket.ObjectRef, uint64, error) {
	return o.obj.GetRootRef(ctx)
}

// AccessWorldState calls cb with a guarded cursor at ref or the object's root.
func (o *guardedObjectState) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return guardedAccess(ctx, ref, cb, unscopedCheck(o.ws, o.guard), o.obj.AccessWorldState)
}

// SetRootRef checks the object.
func (o *guardedObjectState) SetRootRef(ctx context.Context, nref *bucket.ObjectRef) (uint64, error) {
	if err := o.check(ctx, ""); err != nil {
		return 0, err
	}
	return o.obj.SetRootRef(ctx, nref)
}

// ApplyObjectOp checks the op's type and the object. An object op writes only
// the object it applies to. A guard that fails to decide returns a system
// error, so the op is retried rather than rejected.
func (o *guardedObjectState) ApplyObjectOp(ctx context.Context, op Operation, opSender peer.ID) (uint64, bool, error) {
	if op == nil {
		return 0, false, ErrEmptyOp
	}
	if err := o.check(ctx, op.TypeID()); err != nil {
		return 0, IsGuardSysErr(err), err
	}
	return o.obj.ApplyObjectOp(ctx, op, opSender)
}

// IncrementRev checks the object.
func (o *guardedObjectState) IncrementRev(ctx context.Context) (uint64, error) {
	if err := o.check(ctx, ""); err != nil {
		return 0, err
	}
	return o.obj.IncrementRev(ctx)
}

// WaitRev waits on the wrapped object.
func (o *guardedObjectState) WaitRev(ctx context.Context, rev uint64, ignoreNotFound bool) (uint64, error) {
	return o.obj.WaitRev(ctx, rev, ignoreNotFound)
}

// GetModSeqno reads the wrapped object's mod seqno.
func (o *guardedObjectState) GetModSeqno(ctx context.Context) (uint64, error) {
	return o.obj.GetModSeqno(ctx)
}

// Release releases the wrapped object state.
func (o *guardedObjectState) Release() {
	ReleaseObjectState(o.obj)
}

// _ is a type assertion
var (
	_ WorldState                 = (*guardedWorldState)(nil)
	_ ObjectPurger               = (*guardedWorldState)(nil)
	_ ObjectRootRefBatcher       = (*guardedWorldState)(nil)
	_ ObjectBodyBatcher          = (*guardedWorldState)(nil)
	_ ObjectBodyPageBatcher      = (*guardedWorldState)(nil)
	_ ObjectBodyPageSeqnoBatcher = (*guardedWorldState)(nil)
	_ ObjectRevWaiter            = (*guardedWorldState)(nil)
	_ IndexedChangeWatcher       = (*guardedWorldState)(nil)
	_ ObjectState                = (*guardedObjectState)(nil)
	_ WorldStage                 = (*guardedStage)(nil)
	_ Engine                     = (*guardedEngine)(nil)
	_ IndexedChangeWatcher       = (*guardedEngine)(nil)
	_ Tx                         = (*guardedTx)(nil)
	_ RehearsalEngine            = (*guardedRehearsalEngine)(nil)
)
