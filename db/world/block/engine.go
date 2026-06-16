package world_block

import (
	"context"
	"sync"

	trace "github.com/s4wave/spacewave/db/traceutil"

	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// Engine is the world engine instance.
// Uses short-lived block graph transactions internally.
// Reads are against latest state; read txs don't lock.
// Re-tries transaction operations if the underlying transaction is discarded mid-way through.
// Maintains two WorldState objects: one for readers, one for writer.
type Engine struct {
	// le is the logger
	le *logrus.Entry
	// lookupOp looks up a world operation.
	lookupOp world.LookupOp
	// verbose enables verbose logging within world state
	verbose bool
	// wmtx ensures only one write transaction is active at a time
	wmtx csync.Mutex
	// rmtx locks the read-only world instance field & root field & waiters & read/writeTx
	rmtx sync.RWMutex
	// baseRoot is the base root cursor to use.
	// the root cursor is derived with FollowRef from this cursor.
	baseRoot *bucket_lookup.Cursor
	// root is the root cursor in use
	root *bucket_lookup.Cursor
	// readTx is the current read-only world instance
	readTx *Tx
	// writeTx is the current write tx
	// canceled if the state changes mid-write
	writeTx *EngineTx
	// writeTxRel releases wmtx, call when unsetting writeTx
	writeTxRel func()
	// commitFn is a function to be called just before a commit is confirmed.
	// can be nil
	commitFn CommitFn
	// durableHeadRef is the head last written to durable storage via commitFn.
	// Distinct from root, which tracks the current in-memory root. In the
	// single-writer path the durable head is advanced only by Sync, so root can
	// run ahead of durableHeadRef between fences. Guarded by rmtx.
	durableHeadRef *bucket.ObjectRef
	// writeBlockStore is the long-lived block store backing write transactions.
	// In the single-writer path with a store that is not self-buffered it is a
	// deferred BufferedStore wrapping the bucket store: block writes accumulate
	// and become durable only at Sync. Otherwise it is the bucket store directly
	// (coordinator mode stays durable-on-write; self-buffered stores own their
	// own intake + Sync fence). Stable for the engine lifetime because the world
	// cursor never switches buckets.
	writeBlockStore block.StoreOps
	// deferDurability enables the single-writer deferred-durability mode: block
	// writes and the durable head advance batch until Sync instead of becoming
	// durable per commit. Opt-in because callers that propose blocks to a remote
	// authority or write-then-exit need durable-on-write semantics. Ignored when
	// a write coordinator is configured (coordinator mode stays durable-on-write).
	deferDurability bool
	// writeCoordinator gates standalone ObjectStore-backed writers.
	writeCoordinator coord.Coordinator
	// writeCoordScope identifies this engine's durable ObjectStore head.
	writeCoordScope coord.Scope
	// writeCoordKeyPrefix is invalidated when the durable World head changes.
	writeCoordKeyPrefix []byte
	// writeHeadRefresh rereads the durable World head before a write mutation starts.
	writeHeadRefresh func(context.Context) (*bucket.ObjectRef, error)
	// closed is set after Close releases cursor and transaction resources.
	closed bool
}

// CommitFn is a function to call with the updated root before confirming it.
// Should be used to write the updated state back to storage.
// Note: engine rmtx is locked while cb is called, do not block or call engine funcs!
// If an error is returned the change will be rolled back.
// Do not change the nrootBcs during this call.
type CommitFn func(ctx context.Context, baseRef, nref *bucket.ObjectRef) error

// EngineOption configures optional Engine integrations.
type EngineOption func(*Engine)

// WithDeferredDurability enables single-writer deferred durability: block writes
// and the durable head advance batch in memory until Sync. It is the IC-3
// hot-path fence and has no effect when a write coordinator is configured.
func WithDeferredDurability() EngineOption {
	return func(e *Engine) {
		e.deferDurability = true
	}
}

// WithWriteCoordinator requires write transactions to acquire a coordinator
// lease and refresh the durable World head before mutation.
func WithWriteCoordinator(
	coordinator coord.Coordinator,
	scope coord.Scope,
	keyPrefix []byte,
	refreshHead func(context.Context) (*bucket.ObjectRef, error),
) EngineOption {
	return func(e *Engine) {
		e.writeCoordinator = coordinator
		e.writeCoordScope = scope
		e.writeCoordKeyPrefix = append([]byte(nil), keyPrefix...)
		e.writeHeadRefresh = refreshHead
	}
}

// NewEngine constructs a new world engine.
// commitFn can be nil.
func NewEngine(
	ctx context.Context,
	le *logrus.Entry,
	root *bucket_lookup.Cursor,
	lookupOp world.LookupOp,
	commitFn CommitFn,
	verbose bool,
	opts ...EngineOption,
) (*Engine, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/new")
	defer task.End()

	e := &Engine{
		le:             le,
		baseRoot:       root,
		lookupOp:       lookupOp,
		root:           root.Clone(),
		commitFn:       commitFn,
		durableHeadRef: root.GetRef().Clone(),
		verbose:        verbose,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}

	// In the opt-in single-writer deferred-durability mode, defer block durability
	// so per-commit writes accumulate and become durable only at Sync, alongside
	// the deferred durable head. Coordinator mode and all callers that did not opt
	// in stay durable-on-write and use the bucket directly.
	rawWriteStore := e.root.GetBucket()
	e.writeBlockStore = rawWriteStore
	if e.deferDurability && e.writeCoordinator == nil {
		if rawWriteStore.GetSupportedFeatures()&block.StoreFeatureSelfBuffered == 0 {
			// Not self-buffered (e.g. bbolt): defer behind one long-lived
			// BufferedStore that accumulates writes in memory until Sync.
			e.writeBlockStore = block.NewBufferedStore(ctx, rawWriteStore)
		} else {
			// Self-buffered (e.g. blockshard): the store owns its pending buffer
			// and Sync fence, so route commit writes to its background intake
			// rather than the synchronous foreground publish. This is the IC-3
			// OPFS hot path: commits enqueue without a per-commit segment+manifest
			// publish, and Sync fences them durable with the deferred head.
			e.writeBlockStore = newBackgroundWriteStore(rawWriteStore)
		}
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/new/update-read-write-txns")
	err := e.updateReadWriteTxns(taskCtx)
	subtask.End()
	if err != nil {
		return nil, err
	}
	return e, nil
}

// GetRootRef gets the current root cursor reference.
func (e *Engine) GetRootRef() *bucket.ObjectRef {
	e.rmtx.RLock()
	ref := e.root.GetRef().Clone()
	e.rmtx.RUnlock()
	return ref
}

// Sync fences durable storage and advances the durable world head.
//
// The fence is ordered: first the block barrier makes every block written so
// far durable, then the durable head is advanced to the current in-memory root
// via commitFn. Ordering matters because a head that names not-yet-durable
// blocks is the crash window this fence closes.
//
// In the single-writer path (no write coordinator) the per-commit path defers
// the durable head, so this is where it advances. In coordinator mode the head
// is already published per commit, so the head advance here is a no-op and only
// the block barrier runs.
func (e *Engine) Sync(ctx context.Context) (bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/sync")
	defer task.End()

	e.rmtx.Lock()
	defer e.rmtx.Unlock()
	if e.closed {
		return false, errors.New("world block engine is closed")
	}

	// block barrier: drain and fence every buffered block write durable.
	fenced, err := e.writeBlockStore.Sync(ctx)
	if err != nil {
		return false, err
	}

	// advance the durable head, ordered after the block barrier. Skipped in
	// coordinator mode, where each commit already published the head.
	if e.writeCoordinator == nil && e.commitFn != nil {
		cur := e.root.GetRef()
		if e.durableHeadRef == nil || !e.durableHeadRef.EqualsRef(cur) {
			// The root's blocks are durable now (post-barrier); validate it is
			// followable from durable storage before publishing the head. In the
			// single-writer path this is the only validation point, because the
			// per-commit path defers both validation and the head to Sync.
			if err := e.validateRootRefLocked(ctx, cur); err != nil {
				return false, err
			}
			if err := e.commitFn(ctx, e.durableHeadRef, cur.Clone()); err != nil {
				return false, err
			}
			e.durableHeadRef = cur.Clone()
		}
	}
	return fenced, nil
}

// GetGCJournalEntries returns the number of pending GC journal entries.
// Safe to call concurrently. Returns 0 if the read tx or journal is not initialized.
func (e *Engine) GetGCJournalEntries() uint64 {
	e.rmtx.RLock()
	rtx := e.readTx
	e.rmtx.RUnlock()
	if rtx == nil {
		return 0
	}
	return rtx.state.GetGCJournalEntries()
}

// SetRootRef updates the root cursor to point to a new reference.
// Re-creates the internal read transaction with the updated state.
// Cancels any ongoing write tx (to be re-created against new state).
// Can return an error to indicate validation failure.
func (e *Engine) SetRootRef(ctx context.Context, ref *bucket.ObjectRef) error {
	e.rmtx.Lock()
	defer e.rmtx.Unlock()
	if e.closed {
		return errors.New("world block engine is closed")
	}

	return e.setRootRefLocked(ctx, ref)
}

// AdoptRootRefFromWatch updates the root from an advisory coordinator watch
// only when no local write transaction is active. bbolt emits generation events
// for intermediate block writes before the durable World head is updated; watch
// adoption must not roll an in-flight local writer back to the previous head.
func (e *Engine) AdoptRootRefFromWatch(ctx context.Context, ref *bucket.ObjectRef) error {
	e.rmtx.Lock()
	defer e.rmtx.Unlock()
	if e.closed {
		return errors.New("world block engine is closed")
	}
	if e.writeTx != nil {
		return nil
	}
	currentSeqno, ok, err := e.currentRootSeqnoLocked(ctx)
	if err != nil {
		return err
	}
	if ok {
		candidateSeqno, err := e.seqnoForRootRefLocked(ctx, ref)
		if err != nil {
			return err
		}
		if candidateSeqno < currentSeqno {
			return nil
		}
	}
	return e.setRootRefLocked(ctx, ref)
}

// setRootRefLocked updates the root reference while rmtx is locked.
func (e *Engine) setRootRefLocked(ctx context.Context, ref *bucket.ObjectRef) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/set-root-ref")
	defer task.End()

	// if no changes, ignore the call
	if e.root.GetRef().EqualsRef(ref) {
		return nil
	}

	// validate the new root
	if err := ref.Validate(); err != nil {
		return err
	}

	// apply committed changes or rollback
	// oldRoot := e.root.GetRef().Clone()
	oldRoot := e.root
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/set-root-ref/follow-ref")
	nextRoot, err := e.baseRoot.FollowRef(taskCtx, ref)
	subtask.End()
	if err != nil {
		return err
	}
	e.root = nextRoot
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/engine/set-root-ref/update-read-write-txns")
	err = e.updateReadWriteTxns(taskCtx)
	subtask.End()
	if err == nil {
		oldRoot.Release()
	} else {
		e.root = oldRoot
		nextRoot.Release()
	}
	return err
}

// validateRootRefLocked rebuilds the root's WorldState without publishing it.
// Durable head publication must not happen before a fresh cursor can follow the
// full root graph outside the committing transaction.
func (e *Engine) validateRootRefLocked(ctx context.Context, ref *bucket.ObjectRef) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/validate-root-ref")
	defer task.End()

	ws, err := e.worldStateForRootRefLocked(ctx, ref)
	if err != nil {
		return err
	}
	if _, err := ws.objTree.Size(ctx); err != nil {
		ws.Discard()
		return err
	}
	ws.Discard()
	return nil
}

func (e *Engine) seqnoForRootRefLocked(ctx context.Context, ref *bucket.ObjectRef) (uint64, error) {
	ws, err := e.worldStateForRootRefLocked(ctx, ref)
	if err != nil {
		return 0, err
	}
	defer ws.Discard()
	return ws.GetSeqno(ctx)
}

func (e *Engine) worldStateForRootRefLocked(ctx context.Context, ref *bucket.ObjectRef) (*WorldState, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	nextRoot, err := e.baseRoot.FollowRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer nextRoot.Release()

	store := nextRoot.GetBucket()
	xfrm := nextRoot.GetTransformer()
	_, bcs := nextRoot.BuildTransactionWithStore(nil, store)
	ws, err := NewWorldState(
		ctx,
		e.le,
		false,
		nil,
		bcs,
		store,
		xfrm,
		nil,
		e,
		e.lookupOp,
		e.verbose,
	)
	if err != nil {
		return nil, err
	}
	return ws, nil
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
// Check GetReadOnly, might not return a write tx if write=true.
func (e *Engine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	return e.NewBlockEngineTransaction(ctx, write)
}

// NewBlockEngineTransaction returns the world-block specific EngineTx type.
func (e *Engine) NewBlockEngineTransaction(ctx context.Context, write bool) (*EngineTx, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/new-block-engine-transaction")
	defer task.End()

	// writeTx is nil if it's a read-only tx
	if !write {
		e.rmtx.Lock()
		defer e.rmtx.Unlock()
		if e.closed {
			return nil, errors.New("world block engine is closed")
		}
		if e.writeTx == nil && e.writeHeadRefresh != nil {
			if err := e.refreshDurableHeadLocked(ctx); err != nil {
				if e.readTx != nil && isCoordinatedWriteSnapshotError(err) {
					e.readTx.Discard()
					e.readTx = nil
				}
				return nil, err
			}
		}
		if e.writeCoordinator != nil {
			world, err := e.buildWorldState(ctx, true)
			if err != nil {
				return nil, err
			}
			if e.readTx != nil {
				e.readTx.Discard()
				e.readTx = nil
			}
			engTx := newEngineTx(e, nil)
			engTx.readTx = NewTx(world)
			return engTx, nil
		}
		return newEngineTx(e, nil), nil
	}

	// Released in Discard or Commit
	relLock, err := e.wmtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	var lease coord.WriteLease
	if e.writeCoordinator != nil {
		lease, err = e.writeCoordinator.WaitAcquireWriteLease(ctx, e.writeCoordScope)
		if err != nil {
			relLock()
			return nil, err
		}
		if lease == nil {
			relLock()
			return nil, errors.New("world write coordinator returned nil lease")
		}
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/new-block-engine-transaction/build-world-state")
	e.rmtx.Lock()
	defer e.rmtx.Unlock()
	if e.closed {
		if lease != nil {
			_ = lease.Release(ctx)
		}
		relLock()
		return nil, errors.New("world block engine is closed")
	}
	if lease != nil {
		if e.readTx != nil {
			e.readTx.Discard()
			e.readTx = nil
		}
		if _, err := lease.Refresh(taskCtx); err != nil {
			_ = lease.Release(ctx)
			relLock()
			return nil, err
		}
	}
	if e.writeHeadRefresh != nil {
		headRef, err := e.writeHeadRefresh(taskCtx)
		if err != nil {
			if lease != nil {
				_ = lease.Release(ctx)
			}
			relLock()
			return nil, err
		}
		if headRef != nil && !headRef.GetRootRef().GetEmpty() {
			if err := e.setRootRefLocked(taskCtx, headRef); err != nil {
				if lease != nil {
					_ = lease.Release(ctx)
				}
				relLock()
				if lease != nil && isCoordinatedWriteSnapshotError(err) {
					return nil, errors.Wrap(coord.ErrStaleGeneration, "refresh durable head")
				}
				return nil, err
			}
		}
	}
	baseHeadRef := e.root.GetRef().Clone()

	world, err := e.buildWorldState(taskCtx, false)
	subtask.End()
	if err != nil {
		if lease != nil {
			_ = lease.Release(ctx)
		}
		relLock()
		if lease != nil && isCoordinatedWriteSnapshotError(err) {
			return nil, errors.Wrapf(coord.ErrStaleGeneration, "build world state: %v", err)
		}
		return nil, err
	}

	engTx := newEngineTx(e, NewTx(world))
	engTx.baseHeadRef = baseHeadRef
	engTx.lease = lease
	e.writeTx = engTx
	e.writeTxRel = relLock
	return engTx, nil
}

func (e *Engine) refreshDurableHeadLocked(ctx context.Context) error {
	headRef, err := e.writeHeadRefresh(ctx)
	if err != nil || headRef == nil || headRef.GetRootRef().GetEmpty() {
		return err
	}
	currentSeqno, ok, err := e.currentRootSeqnoLocked(ctx)
	if err != nil {
		return err
	}
	if ok {
		candidateSeqno, err := e.seqnoForRootRefLocked(ctx, headRef)
		if err != nil {
			return err
		}
		if candidateSeqno < currentSeqno {
			return nil
		}
	}
	return e.setRootRefLocked(ctx, headRef)
}

func (e *Engine) currentRootSeqnoLocked(ctx context.Context) (uint64, bool, error) {
	if e.readTx != nil {
		seqno, err := e.readTx.GetSeqno(ctx)
		return seqno, true, err
	}
	ref := e.root.GetRef()
	if ref == nil || ref.GetRootRef().GetEmpty() {
		return 0, false, nil
	}
	seqno, err := e.seqnoForRootRefLocked(ctx, ref)
	return seqno, true, err
}

// ForkBlockTransaction forks the transaction at the current state.
func (e *Engine) ForkBlockTransaction(ctx context.Context, write bool) (*Tx, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction")
	defer task.End()

	_, subtask := trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction/read-lock")
	e.rmtx.RLock()
	subtask.End()
	defer e.rmtx.RUnlock()
	if e.closed {
		return nil, errors.New("world block engine is closed")
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction/build-world-state")
	ws, err := e.buildWorldState(taskCtx, !write)
	subtask.End()
	if err != nil {
		return nil, err
	}
	_, subtask = trace.NewTask(ctx, "hydra/world-block/engine/fork-block-transaction/new-tx")
	tx := NewTx(ws)
	subtask.End()
	return tx, nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the WorldState.
// Be sure to call Release on the cursor when done.
func (e *Engine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	e.rmtx.RLock()
	defer e.rmtx.RUnlock()
	if e.closed {
		return nil, errors.New("world block engine is closed")
	}
	ncs := e.baseRoot.Clone()
	ncs.SetRootRef(nil)
	return ncs, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
//
// NOTE: this is the implementation of AccessWorldState for the world/block engine.
func (e *Engine) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	e.rmtx.RLock()
	closed := e.closed
	e.rmtx.RUnlock()
	if closed {
		return errors.New("world block engine is closed")
	}
	if ref == nil {
		ncs := e.root.Clone()
		defer ncs.Release()
		return cb(ncs)
	}

	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// follow the root block ref
	ncs, err := e.root.FollowRef(subCtx, ref)
	if err != nil {
		return err
	}
	defer ncs.Release()

	return cb(ncs)
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (e *Engine) GetSeqno(ctx context.Context) (uint64, error) {
	e.rmtx.Lock()
	defer e.rmtx.Unlock()
	if e.closed {
		return 0, errors.New("world block engine is closed")
	}
	seqno, ok, err := e.currentRootSeqnoLocked(ctx)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	return seqno, nil
}

// WaitSeqno waits for the seqno of the world state to be >= value.
// Returns the seqno when the condition is reached.
// If value == 0, this might return immediately unconditionally.
func (e *Engine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	if e.writeCoordinator != nil {
		// In coordinator mode the local read snapshot does not advance on its
		// own; new seqnos appear only after other writers commit. Wait on the
		// coordinator's commit-generation events (BroadcastChannel-backed in the
		// browser) rather than polling: any seqno advance is a commit that bumps
		// the generation, so re-checking GetSeqno on each event is miss-free.
		// Baseline at the current generation so a commit racing watch setup is
		// still delivered.
		snapshot, err := e.writeCoordinator.Snapshot(ctx, e.writeCoordScope)
		if err != nil {
			return 0, err
		}
		watch, err := e.writeCoordinator.Watch(ctx, e.writeCoordScope, snapshot.Generation)
		if err != nil {
			return 0, err
		}
		defer watch.Close()
		for {
			seqno, err := e.GetSeqno(ctx)
			if err != nil {
				return 0, err
			}
			if seqno >= value {
				return seqno, nil
			}
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case _, ok := <-watch.Events():
				if !ok {
					// The coordinator closes the events channel only when this
					// wait's context is canceled, so report cancellation rather
					// than a spurious watch error.
					if err := ctx.Err(); err != nil {
						return 0, err
					}
					return 0, errors.New("world write coordinator watch closed")
				}
			}
		}
	}

	for {
		e.rmtx.RLock()
		if e.closed {
			e.rmtx.RUnlock()
			return 0, errors.New("world block engine is closed")
		}
		readTx := e.readTx
		e.rmtx.RUnlock()

		seqno, err := readTx.WaitSeqno(ctx, value)
		if readTx.state.discarded.Load() {
			// readTxn was discarded, get the new one.
			continue
		}
		if err != nil {
			return 0, err
		}

		if seqno >= value {
			return seqno, nil
		}
	}
}

// Close releases root cursors and active transaction state owned by the engine.
func (e *Engine) Close() error {
	if e == nil {
		return nil
	}
	e.rmtx.Lock()
	if e.closed {
		e.rmtx.Unlock()
		return nil
	}
	e.closed = true
	if e.writeTx != nil {
		e.writeTx.discardLocked()
	}
	if e.readTx != nil {
		e.readTx.Discard()
		e.readTx = nil
	}
	if e.root != nil {
		e.root.Release()
		e.root = nil
	}
	if e.baseRoot != nil {
		e.baseRoot.Release()
		e.baseRoot = nil
	}
	e.rmtx.Unlock()
	return nil
}

// updateReadWriteTxns updates the readTx and cancels writeTx if the state changed
// expects caller to hold rmtx lock
// the state has been affected only if nil is returned
func (e *Engine) updateReadWriteTxns(ctx context.Context) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/update-read-write-txns")
	defer task.End()

	// This is the only place readTx might be nil (on first call).
	// If no changes have occurred...
	if e.readTx != nil && e.readTx.state.GetRootRef().EqualsRef(e.root.GetRef().GetRootRef()) {
		return nil
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/update-read-write-txns/build-world-state")
	world, err := e.buildWorldState(taskCtx, true)
	subtask.End()
	if err != nil {
		return err
	}
	// cancel the old write tx if active
	if e.writeTx != nil {
		e.writeTx.discardLocked()
		e.writeTx = nil // field is checked during Commit() as well
	}
	// swap in the new read tx
	readTx := NewTx(world)
	if e.readTx != nil {
		e.readTx.Discard()
	}
	e.readTx = readTx
	return nil
}

// buildWorldState builds the world state transaction and cursor fields.
// expects caller to hold rmtx
func (e *Engine) buildWorldState(ctx context.Context, readOnly bool) (*WorldState, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/engine/build-world-state")
	defer task.End()

	_, subtask := trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/get-bucket")
	// Both read and write transactions read and write through writeBlockStore so
	// that, in the single-writer deferred path, reads see blocks that are
	// committed in memory but not yet drained to durable storage (read your
	// writes before Sync). It is the bucket store directly in coordinator and
	// self-buffered modes.
	store := e.writeBlockStore
	worldStore := store
	if !readOnly && e.writeCoordinator != nil {
		worldStore = nil
	}
	subtask.End()
	_, subtask = trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/get-transformer")
	xfrm := e.root.GetTransformer()
	subtask.End()
	_, subtask = trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/build-transaction")
	btx, bcs := e.root.BuildTransactionWithStore(nil, store)
	subtask.End()
	if readOnly {
		btx = nil
	}
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/engine/build-world-state/new-world-state")
	ws, err := NewWorldState(
		taskCtx,
		e.le,
		!readOnly,
		btx, bcs,
		worldStore,
		xfrm,
		nil,
		e,
		e.lookupOp,
		e.verbose,
	)
	subtask.End()
	if err != nil {
		return nil, err
	}
	return ws, nil
}

// _ is a type assertion
var _ world.Engine = ((*Engine)(nil))
