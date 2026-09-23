package repofs

import (
	"context"
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/util/routine"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	hydra_git "github.com/s4wave/spacewave/db/git"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/world"
)

// Engine builds repo transactions against one world object.
type Engine struct {
	// ctx is the engine lifecycle context.
	ctx context.Context
	// ws is the source world state.
	ws world.WorldState
	// obj is the git repo object state.
	obj world.ObjectState

	// mtx guards changeCbs.
	mtx sync.Mutex
	// changeCbs stores repo invalidation callbacks.
	changeCbs map[uint64]func()
	// nextChange is the next callback identifier.
	nextChange atomic.Uint64
	// watchRev is the revision baseline for the active watcher.
	watchRev atomic.Uint64
	// watchRoutine owns the object revision watcher.
	watchRoutine *routine.RoutineContainer
	// cancel stops the engine lifecycle.
	cancel context.CancelFunc
}

// NewEngine constructs a repo filesystem engine.
func NewEngine(ctx context.Context, ws world.WorldState, obj world.ObjectState) *Engine {
	// Keep the watcher within the engine's explicit Close lifecycle.
	watchCtx, cancel := context.WithCancel(ctx)
	watchRoutine := routine.NewRoutineContainer()
	watchRoutine.SetContext(watchCtx, false)

	// Transactions and callbacks share this object state.
	return &Engine{
		ctx:          watchCtx,
		ws:           ws,
		obj:          obj,
		watchRoutine: watchRoutine,
		cancel:       cancel,
	}
}

// NewTransaction opens a repo transaction.
func (e *Engine) NewTransaction(ctx context.Context, write bool) (hydra_git.Tx, error) {
	// Resolve the current repository snapshot.
	objRef, _, err := e.obj.GetRootRef(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get root ref")
	}

	// Retain the storage cursors until the transaction is discarded.
	rootCursor, err := e.ws.BuildStorageCursor(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "build storage cursor")
	}
	locCursor, err := rootCursor.FollowRef(ctx, objRef)
	if err != nil {
		rootCursor.Release()
		return nil, errors.Wrap(err, "follow ref")
	}

	// Expose a writable transaction only when requested.
	var btx *block.Transaction
	var bcs *block.Cursor
	if write {
		btx, bcs = locCursor.BuildTransaction(nil)
	}
	if !write {
		_, bcs = locCursor.BuildTransaction(nil)
	}

	// Validate the repository before exposing its Git store.
	repob, err := git_block.UnmarshalRepo(ctx, bcs)
	if err != nil {
		locCursor.Release()
		rootCursor.Release()
		return nil, errors.Wrap(err, "unmarshal repo")
	}
	if err := repob.Validate(); err != nil {
		locCursor.Release()
		rootCursor.Release()
		return nil, errors.Wrap(err, "validate repo")
	}

	// Transfer both cursors to the successfully opened transaction.
	store, err := git_block.NewStore(ctx, btx, bcs, &memory.IndexStorage{}, nil)
	if err != nil {
		locCursor.Release()
		rootCursor.Release()
		return nil, errors.Wrap(err, "create git store")
	}

	return &projectionTx{
		Store:      store,
		obj:        e.obj,
		rootCursor: rootCursor,
		locCursor:  locCursor,
	}, nil
}

// Close releases the repo filesystem engine lifecycle.
func (e *Engine) Close() {
	e.watchRoutine.ClearContext()
	e.cancel()
}

// AddDotGitChangeCb registers a repo-level change callback.
// Callbacks run in registration order, invalidating parents before descendants.
func (e *Engine) AddDotGitChangeCb(cb func()) func() {
	// A nil callback needs no watcher registration.
	if cb == nil {
		return func() {}
	}

	// Retain the callback and detect the first active subscriber.
	id := e.nextChange.Add(1)
	var startWatch bool
	e.mtx.Lock()
	if e.changeCbs == nil {
		e.changeCbs = make(map[uint64]func())
	}
	e.changeCbs[id] = cb
	if len(e.changeCbs) == 1 {
		startWatch = true
	}
	e.mtx.Unlock()

	// The first subscriber starts revision tracking for this engine.
	if startWatch {
		_, rev, err := e.obj.GetRootRef(e.ctx)
		if err != nil {
			e.mtx.Lock()
			delete(e.changeCbs, id)
			e.mtx.Unlock()
			cb()
			return func() {}
		}
		e.watchRev.Store(rev)
		e.watchRoutine.SetRoutine(e.watchChanges)
	}

	// The final unsubscribe stops revision tracking.
	return func() {
		var stopWatch bool
		e.mtx.Lock()
		delete(e.changeCbs, id)
		if len(e.changeCbs) == 0 {
			stopWatch = true
		}
		e.mtx.Unlock()
		if stopWatch {
			e.watchRoutine.SetRoutine(nil)
		}
	}
}

// watchChanges invalidates repository cursors when the object revision advances.
func (e *Engine) watchChanges(ctx context.Context) error {
	// Wait from the subscriber baseline without polling the object state.
	rev := e.watchRev.Load()
	for {
		// A failed watch invalidates its snapshots unless Close canceled it.
		nextRev, err := e.obj.WaitRev(ctx, rev+1, false)
		if err != nil {
			if ctx.Err() == nil {
				e.callChangeCbs()
			}
			return nil
		}

		// Publish invalidation before waiting for a later revision.
		rev = nextRev
		e.callChangeCbs()
	}
}

// callChangeCbs invalidates older cursors before notifying newer descendants.
func (e *Engine) callChangeCbs() {
	// Snapshot callbacks in registration order; map order can notify a child
	// while its cached parent still exposes the previous repository snapshot.
	e.mtx.Lock()
	cbs := make([]func(), 0, len(e.changeCbs))
	for _, id := range slices.Sorted(maps.Keys(e.changeCbs)) {
		cbs = append(cbs, e.changeCbs[id])
	}
	e.mtx.Unlock()

	// Callbacks may unsubscribe or release the engine, so run outside the lock.
	for _, cb := range cbs {
		cb()
	}
}

// projectionTx publishes a Git store root through its world object.
type projectionTx struct {
	// Store implements the Git storage operations.
	*git_block.Store

	// obj receives the committed repository root.
	obj world.ObjectState
	// rootCursor retains the storage root for the transaction lifetime.
	rootCursor *bucket_lookup.Cursor
	// locCursor retains the repository location for the transaction lifetime.
	locCursor *bucket_lookup.Cursor

	// once releases the store and its cursors on the first Discard call.
	once sync.Once
}

// Commit writes the store before publishing its new object root.
func (t *projectionTx) Commit(ctx context.Context) error {
	// Finalize Git storage before making its root visible to readers.
	if err := t.Store.Commit(); err != nil {
		return err
	}
	if t.GetReadOnly() {
		return nil
	}

	// Publishing the object root wakes repository revision subscribers.
	nextRef := t.locCursor.GetRef()
	nextRef.RootRef = t.Store.GetRef().Clone()
	_, err := t.obj.SetRootRef(ctx, nextRef)
	return err
}

// Discard releases the store and both cursors exactly once.
func (t *projectionTx) Discard() {
	t.once.Do(func() {
		// Store.Close only releases cached packs and cancels its context.
		// It always returns nil.
		_ = t.Close()
		t.locCursor.Release()
		t.rootCursor.Release()
	})
}

// _ is a type assertion
var (
	_ hydra_git.Engine = (*Engine)(nil)
	_ hydra_git.Tx     = (*projectionTx)(nil)
)
