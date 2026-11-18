package world

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"

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
// An op applied through a guarded state is guarded inside the engine: the
// guarded state checks the op's type, then applies it with the guard on the
// context, and the engine hands the op its own state wrapped by
// GuardOpWorldState or GuardOpObjectState. Invariant: every write an op makes
// while applied through a guarded state, including object and graph writes,
// type writes, nested ops and object ops, is checked by the guard against the
// key it actually writes. An apply with no guard on its context hands the op
// the engine's state unchanged.
//
// A guarded apply fails closed. When the guard refused or failed to decide any
// write in the op's tree of nested ops, the apply returns that error, even
// when the op swallowed it and returned success: the op did not do what it
// believes it did. An apply whose engine never wrapped its state for the
// guard returns ErrEngineUnguardedOp. A guarded transaction discards itself
// when an op applied in it fails, as guardedTx describes.
//
// The guarded types spell out every method of the interfaces they implement,
// so a method added to an interface fails to compile until it is guarded.
//
// A WriteGuard must be comparable, such as a pointer: guarded states compare
// guards to avoid checking a write twice.
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
	// Create marks a write that only creates the objects ObjectKeys names.
	Create bool
	// Quad is the graph quad the write sets or deletes, or nil.
	Quad GraphQuad
	// Unscoped marks a write that can change objects ObjectKeys does not
	// name: a writable graph handle, a rename of an object's descendants, and
	// a block removal, root move or completion proof through a guarded cursor.
	Unscoped bool
}

// IsGuardSysErr reports whether err, returned by a WriteGuard or recorded in
// a guarded apply, is a system error rather than a refusal: only an
// *OperationRejection and ErrEngineUnguardedOp, which a retry meets again,
// are final.
func IsGuardSysErr(err error) bool {
	var rejection *OperationRejection
	return err != nil && !errors.As(err, &rejection) && !errors.Is(err, ErrEngineUnguardedOp)
}

// WorldStateGetter is an ObjectState that returns the WorldState holding it,
// so a component that receives only the object can guard writes through it
// and write beside the object in the same transaction. A handle whose writes
// commit in a transaction of their own, such as an engine's object, does not
// implement it.
type WorldStateGetter interface {
	// GetWorldState returns the WorldState that holds the object, guarded as
	// the object is.
	GetWorldState() WorldState
}

// ErrEngineUnguardedOp is returned by a guarded apply whose engine applied the
// op without wrapping its state by GuardOpWorldState or GuardOpObjectState.
var ErrEngineUnguardedOp = errors.New("world: engine does not guard the writes of ops it applies")

// opApplyKey is the context key of the opApply an engine guards an op under.
type opApplyKey struct{}

// opApply is one op applied through a guarded state, carried on the context
// to the engine that applies it.
//
// Invariant: an op applied through guarded layers stacked on one engine
// state, such as a component's guard over the handle an engine gave an op,
// creates exactly one opApply, and the engine takes it. The layers fold: each
// checks the op's type, then hands its guard to the layer it wraps, and only
// the innermost, which wraps the engine's state, creates the apply with every
// layer's guard. So each write the op makes is checked by every layer's
// guard, and an apply no engine takes is an engine that does not guard, never
// a layer directly above another layer.
//
// A layer over a state that is not a guarded layer, such as an engine's state
// over a guarded engine, does not fold: it creates its own apply, and a
// guarded layer the state applies the op through creates another, nested in
// it, which the engine takes. The nested apply checks the outer guards too,
// but the outer apply is never taken, so the op fails closed with
// ErrEngineUnguardedOp, recorded in the op tree so an op that swallows it
// fails too. GuardWorldState adds no layer where the guard already guards, so
// a mount's state over its own guarded engine stays one layer.
type opApply struct {
	// guards are the guards the op's writes are checked by, each once, in no
	// promised order.
	guards []WriteGuard
	// taken is set when an engine wraps its state for the guards.
	taken atomic.Bool
	// refusal holds the first guard error in the op's tree of nested ops.
	refusal *opRefusal
}

// opRefusal holds the first error a guard returned during an op tree.
type opRefusal struct {
	mtx sync.Mutex
	err error
}

// newOpApply returns the apply of an op through states stacked with guards, as
// a nested op of parent when parent is set: it keeps the parent's guards and
// shares its refusal, so an outer op that swallows a nested refusal fails too.
func newOpApply(parent *opApply, guards []WriteGuard) *opApply {
	// Start from the parent's guards and refusal, or a refusal of its own.
	apply := &opApply{refusal: &opRefusal{}}
	if parent != nil {
		apply.guards, apply.refusal = slices.Clip(parent.guards), parent.refusal
	}

	// Add each guard the apply does not hold yet.
	held := make(map[WriteGuard]struct{}, len(apply.guards)+len(guards))
	for _, guard := range apply.guards {
		held[guard] = struct{}{}
	}
	for _, guard := range guards {
		if _, ok := held[guard]; !ok {
			held[guard] = struct{}{}
			apply.guards = append(apply.guards, guard)
		}
	}
	return apply
}

// nestIn returns the parent and guards of the apply the innermost layer
// creates under ctx, given the op whose handle a layer is as parent and the
// layers' guards: nested in the op of ctx when no layer is a handle. When a
// layer is a handle and ctx carries the apply of another op, a guarded layer
// above a state the fold does not see through, the apply keeps that op's
// guards too, so every guard above still checks each write.
func nestIn(ctx context.Context, parent *opApply, guards []WriteGuard) (*opApply, []WriteGuard) {
	outer := getOpApply(ctx)
	switch {
	case parent == nil:
		return outer, guards
	case outer != nil && outer != parent:
		return parent, append(guards, outer.guards...)
	}
	return parent, guards
}

// getOpApply returns the apply ctx carries, or nil.
func getOpApply(ctx context.Context) *opApply {
	apply, _ := ctx.Value(opApplyKey{}).(*opApply)
	return apply
}

// record keeps err, a guard's error for a write made during the apply, and
// returns it. A nil apply, a write outside any op, records nothing.
func (a *opApply) record(err error) error {
	// A write outside any op, or one the guard allows, records nothing.
	if a == nil || err == nil {
		return err
	}

	// Keep the first error of the op tree.
	a.refusal.mtx.Lock()
	if a.refusal.err == nil {
		a.refusal.err = err
	}
	a.refusal.mtx.Unlock()
	return err
}

// guardErr returns the first guard error recorded in the op tree of a, or nil
// when a is nil or none was recorded.
func (a *opApply) guardErr() error {
	if a == nil {
		return nil
	}
	a.refusal.mtx.Lock()
	defer a.refusal.mtx.Unlock()
	return a.refusal.err
}

// finish returns the result of an apply that returned seqno, sysErr and err,
// failing it closed: a guard error recorded in the op tree, even one the op
// swallowed, is returned in place of the op's result, and a success no engine
// guarded returns ErrEngineUnguardedOp. A guard that failed to decide stays a
// system error, retried rather than rejected. Any failure calls discard.
func (a *opApply) finish(seqno uint64, sysErr bool, err error, discard func()) (uint64, bool, error) {
	// Fail with a recorded guard error, or a success no engine guarded.
	gerr := a.guardErr()
	switch {
	case gerr != nil:
		err, sysErr = gerr, IsGuardSysErr(gerr)
	case err == nil && !a.taken.Load():
		err, sysErr = a.record(ErrEngineUnguardedOp), false
	case err == nil:
		return seqno, sysErr, nil
	}

	// Discard the transaction the failed op wrote into.
	if discard != nil {
		discard()
	}
	return 0, sysErr, err
}

// GuardOpWorldState returns the handle an engine gives an op it applies to its
// state ws under ctx: ws guarded by every guard of the apply ctx carries, or
// ws when ctx carries none. An engine calls it where it hands its own state to
// an op; a guarded apply whose engine does not fails.
func GuardOpWorldState(ctx context.Context, ws WorldState) WorldState {
	// Outside a guarded apply, hand the op ws unchanged.
	apply := getOpApply(ctx)
	if apply == nil {
		return ws
	}

	// Mark the apply guarded, then wrap ws in each guard.
	apply.taken.Store(true)
	for _, guard := range apply.guards {
		ws = &guardedWorldState{ws: ws, guard: guard, apply: apply}
	}
	return ws
}

// GuardOpResult returns the result an engine reports for an op it applied
// under ctx, through GuardOpWorldState or GuardOpObjectState, that returned
// sysErr and err: a success becomes the first guard error recorded in the
// op's tree, even one the op swallowed. An engine calls it before its
// transaction can commit, so no engine path commits a refused op.
func GuardOpResult(ctx context.Context, sysErr bool, err error) (bool, error) {
	if err != nil {
		return sysErr, err
	}
	if gerr := getOpApply(ctx).guardErr(); gerr != nil {
		return IsGuardSysErr(gerr), gerr
	}
	return sysErr, nil
}

// GuardOpObjectState returns the handle an engine gives an object op it
// applies to obj, held by ws, under ctx, as GuardOpWorldState does. Each
// layer wraps the one before it, so releasing the result releases obj.
func GuardOpObjectState(ctx context.Context, obj ObjectState, ws WorldState) ObjectState {
	// Outside a guarded apply, hand the op obj unchanged.
	apply := getOpApply(ctx)
	if apply == nil {
		return obj
	}

	// Mark the apply guarded, then wrap obj in each guard.
	apply.taken.Store(true)
	for _, guard := range apply.guards {
		obj = &guardedObjectState{obj: obj, ws: ws, guard: guard, apply: apply}
	}
	return obj
}

// GuardWorldState returns ws with every write checked by guard first.
// A nil guard returns ws, and so does a ws guard already guards: a guarded
// state, or an engine's state whose engine guard guards, which makes each
// write in a transaction the guard checks.
func GuardWorldState(ws WorldState, guard WriteGuard) WorldState {
	if guard == nil || ws == nil || guardedBy(ws) == guard {
		return ws
	}
	return &guardedWorldState{ws: ws, guard: guard}
}

// guardedBy returns the guard that checks every write made through ws, or nil
// when ws is not guarded as a whole.
func guardedBy(ws WorldState) WriteGuard {
	switch g := ws.(type) {
	case *guardedWorldState:
		return g.guard
	case *guardedTx:
		return g.guard
	case *engineWorldState:
		if e, ok := g.e.(*guardedEngine); ok {
			return e.guard
		}
		if e, ok := g.e.(*guardedRehearsalEngine); ok {
			return e.guard
		}
	}
	return nil
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
	return &guardedTx{guardedWorldState: &guardedWorldState{ws: t, guard: e.guard, opFailed: t.Discard}, tx: t}
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

// guardedTx is a transaction whose writes the guard checks. An op that fails
// in it, World or object op, discards it. The op log records only an op that
// succeeds, so a failed op's earlier writes, left in the transaction, would
// commit a World that no replay of the log rebuilds.
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
	// opFailed, when set, discards the transaction an op applied through the
	// state or its objects failed closed in.
	opFailed func()
	// apply is the op whose handle the state is, set by GuardOpWorldState, or
	// nil for a state handed to a component directly.
	apply *opApply
}

// check describes w to the guard and records its error in the op.
func (s *guardedWorldState) check(ctx context.Context, w *GuardedWrite) error {
	return s.apply.record(s.guard.CheckWrite(ctx, s.ws, w))
}

// unscoped returns the check a guarded cursor makes before an unscoped write.
func (s *guardedWorldState) unscoped() func(context.Context) error {
	return func(ctx context.Context) error {
		return s.check(ctx, &GuardedWrite{Unscoped: true})
	}
}

// object wraps an object state returned by the wrapped state.
func (s *guardedWorldState) object(obj ObjectState, err error) (ObjectState, error) {
	if obj == nil {
		return obj, err
	}
	return &guardedObjectState{obj: obj, ws: s.ws, guard: s.guard, opFailed: s.opFailed, apply: s.apply}, err
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
	return guardedStorageCursor(ctx, s.unscoped(), s.ws.BuildStorageCursor)
}

// AccessWorldState calls cb with a guarded cursor on the wrapped state's
// storage.
func (s *guardedWorldState) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return guardedAccess(ctx, ref, cb, s.unscoped(), s.ws.AccessWorldState)
}

// StageWorldState opens a guarded stage on the wrapped state.
func (s *guardedWorldState) StageWorldState(ctx context.Context) (WorldStage, error) {
	stage, err := s.ws.StageWorldState(ctx)
	return guardStage(stage, err, s.unscoped())
}

// GetObject returns a guarded object state.
func (s *guardedWorldState) GetObject(ctx context.Context, key string) (ObjectState, bool, error) {
	obj, found, err := s.ws.GetObject(ctx, key)
	if err != nil || !found || obj == nil {
		ReleaseObjectState(obj)
		return nil, found, err
	}
	return &guardedObjectState{obj: obj, ws: s.ws, guard: s.guard, opFailed: s.opFailed, apply: s.apply}, true, nil
}

// IterateObjects iterates keys through the wrapped state; it yields no
// object state.
func (s *guardedWorldState) IterateObjects(ctx context.Context, prefix string, reversed bool) ObjectIterator {
	return s.ws.IterateObjects(ctx, prefix, reversed)
}

// CreateObject checks the new key.
func (s *guardedWorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (ObjectState, error) {
	if err := s.check(ctx, &GuardedWrite{ObjectKeys: []string{key}, Create: true}); err != nil {
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

// ApplyWorldOp checks the op's type, then applies it with the guard on the
// context, so the engine guards the handle it gives the op and the guard
// checks each write the op makes. The apply fails closed as WriteGuard
// describes. A guard that fails to decide returns a system error, so the op
// is retried rather than rejected.
func (s *guardedWorldState) ApplyWorldOp(ctx context.Context, op Operation, opSender peer.ID) (uint64, bool, error) {
	if op == nil {
		return 0, false, ErrEmptyOp
	}
	return s.applyWorldOp(ctx, op, opSender, nil, nil, nil)
}

// applyWorldOp applies op through this layer under the guarded layers stacked
// above it, which pass their guards as above, the op whose handle the nearest
// of them is as parent, and the discard of the transaction they write into.
// It folds as opApply describes. The layers wrap one transaction, so a failed
// op discards it once, through the innermost layer that can.
func (s *guardedWorldState) applyWorldOp(ctx context.Context, op Operation, opSender peer.ID, parent *opApply, above []WriteGuard, discard func()) (uint64, bool, error) {
	// Check the op's type.
	if err := s.check(ctx, &GuardedWrite{OpTypeID: op.TypeID()}); err != nil {
		return 0, IsGuardSysErr(err), err
	}

	// Hand the guard to a guarded layer below, nested in the op whose handle
	// the innermost layer is.
	if s.apply != nil {
		parent = s.apply
	}
	if s.opFailed != nil {
		discard = s.opFailed
	}
	guards := append(slices.Clip(above), s.guard)
	if inner := guardedLayer(s.ws); inner != nil {
		return inner.applyWorldOp(ctx, op, opSender, parent, guards, discard)
	}

	// Over the engine's state, apply it with each of its writes checked by
	// every layer's guard, nested in the op of ctx when no layer is a handle.
	apply := newOpApply(nestIn(ctx, parent, guards))
	seqno, sysErr, err := s.ws.ApplyWorldOp(context.WithValue(ctx, opApplyKey{}, apply), op, opSender)
	return apply.finish(seqno, sysErr, err, discard)
}

// guardedLayer returns ws as a guarded layer, or nil when it is not one.
func guardedLayer(ws WorldState) *guardedWorldState {
	switch g := ws.(type) {
	case *guardedWorldState:
		return g
	case *guardedTx:
		return g.guardedWorldState
	}
	return nil
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
	// opFailed, when set, discards the transaction an op applied to the
	// object failed closed in.
	opFailed func()
	// apply is the op whose handle the object is, or nil.
	apply *opApply
}

// check describes a write to this object to the guard and records its error
// in the op.
func (o *guardedObjectState) check(ctx context.Context, opTypeID string) error {
	w := &GuardedWrite{OpTypeID: opTypeID, ObjectKeys: []string{o.obj.GetKey()}}
	return o.apply.record(o.guard.CheckWrite(ctx, o.ws, w))
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
	unscoped := func(ctx context.Context) error {
		return o.apply.record(o.guard.CheckWrite(ctx, o.ws, &GuardedWrite{Unscoped: true}))
	}
	return guardedAccess(ctx, ref, cb, unscoped, o.obj.AccessWorldState)
}

// SetRootRef checks the object.
func (o *guardedObjectState) SetRootRef(ctx context.Context, nref *bucket.ObjectRef) (uint64, error) {
	if err := o.check(ctx, ""); err != nil {
		return 0, err
	}
	return o.obj.SetRootRef(ctx, nref)
}

// ApplyObjectOp checks the op's type and the object, then applies it with the
// guard on the context, as guardedWorldState.ApplyWorldOp does. A guard that
// fails to decide returns a system error, so the op is retried rather than
// rejected.
func (o *guardedObjectState) ApplyObjectOp(ctx context.Context, op Operation, opSender peer.ID) (uint64, bool, error) {
	if op == nil {
		return 0, false, ErrEmptyOp
	}
	return o.applyObjectOp(ctx, op, opSender, nil, nil, nil)
}

// applyObjectOp applies op through this layer under the guarded layers stacked
// above it, folding and discarding as guardedWorldState.applyWorldOp does.
func (o *guardedObjectState) applyObjectOp(ctx context.Context, op Operation, opSender peer.ID, parent *opApply, above []WriteGuard, discard func()) (uint64, bool, error) {
	// Check the op's type and the object.
	if err := o.check(ctx, op.TypeID()); err != nil {
		return 0, IsGuardSysErr(err), err
	}

	// Hand the guard to a guarded layer below.
	if o.apply != nil {
		parent = o.apply
	}
	if o.opFailed != nil {
		discard = o.opFailed
	}
	guards := append(slices.Clip(above), o.guard)
	if inner, ok := o.obj.(*guardedObjectState); ok {
		return inner.applyObjectOp(ctx, op, opSender, parent, guards, discard)
	}

	// Over the engine's object, apply it with every layer's guard.
	apply := newOpApply(nestIn(ctx, parent, guards))
	rev, sysErr, err := o.obj.ApplyObjectOp(context.WithValue(ctx, opApplyKey{}, apply), op, opSender)
	return apply.finish(rev, sysErr, err, discard)
}

// GetWorldState returns the World that holds the object, guarded as the object
// is.
func (o *guardedObjectState) GetWorldState() WorldState {
	ws := o.ws
	if holder, ok := o.obj.(WorldStateGetter); ok {
		ws = holder.GetWorldState()
	}
	return &guardedWorldState{ws: ws, guard: o.guard, opFailed: o.opFailed, apply: o.apply}
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
	_ WorldStateGetter           = (*guardedObjectState)(nil)
	_ WorldStage                 = (*guardedStage)(nil)
	_ Engine                     = (*guardedEngine)(nil)
	_ IndexedChangeWatcher       = (*guardedEngine)(nil)
	_ Tx                         = (*guardedTx)(nil)
	_ RehearsalEngine            = (*guardedRehearsalEngine)(nil)
)
