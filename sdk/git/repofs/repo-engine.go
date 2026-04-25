package repofs

import (
	"context"
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
	watchCtx, cancel := context.WithCancel(ctx)
	watchRoutine := routine.NewRoutineContainer()
	watchRoutine.SetContext(watchCtx, false)
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
	objRef, _, err := e.obj.GetRootRef(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get root ref")
	}

	rootCursor, err := e.ws.BuildStorageCursor(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "build storage cursor")
	}
	locCursor, err := rootCursor.FollowRef(ctx, objRef)
	if err != nil {
		rootCursor.Release()
		return nil, errors.Wrap(err, "follow ref")
	}

	var btx *block.Transaction
	var bcs *block.Cursor
	if write {
		btx, bcs = locCursor.BuildTransaction(nil)
	}
	if !write {
		_, bcs = locCursor.BuildTransaction(nil)
	}

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
func (e *Engine) AddDotGitChangeCb(cb func()) func() {
	if cb == nil {
		return func() {}
	}

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

func (e *Engine) watchChanges(ctx context.Context) error {
	rev := e.watchRev.Load()
	for {
		nextRev, err := e.obj.WaitRev(ctx, rev+1, false)
		if err != nil {
			if ctx.Err() == nil {
				e.callChangeCbs()
			}
			return nil
		}
		rev = nextRev
		e.callChangeCbs()
	}
}

func (e *Engine) callChangeCbs() {
	e.mtx.Lock()
	cbs := make([]func(), 0, len(e.changeCbs))
	for _, cb := range e.changeCbs {
		cbs = append(cbs, cb)
	}
	e.mtx.Unlock()

	for _, cb := range cbs {
		cb()
	}
}

// _ is a type assertion
var _ hydra_git.Engine = ((*Engine)(nil))

type projectionTx struct {
	*git_block.Store

	obj        world.ObjectState
	rootCursor *bucket_lookup.Cursor
	locCursor  *bucket_lookup.Cursor

	once sync.Once
}

func (t *projectionTx) Commit(ctx context.Context) error {
	if err := t.Store.Commit(); err != nil {
		return err
	}
	if t.GetReadOnly() {
		return nil
	}

	nextRef := t.locCursor.GetRef()
	nextRef.RootRef = t.Store.GetRef().Clone()
	_, err := t.obj.SetRootRef(ctx, nextRef)
	return err
}

func (t *projectionTx) Discard() {
	t.once.Do(func() {
		_ = t.Store.Close()
		t.locCursor.Release()
		t.rootCursor.Release()
	})
}

// _ is a type assertion
var _ hydra_git.Tx = ((*projectionTx)(nil))
