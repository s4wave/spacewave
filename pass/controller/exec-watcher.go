package pass_controller

import (
	"context"

	forge_execution "github.com/aperturerobotics/forge/execution"
	forge_pass "github.com/aperturerobotics/forge/pass"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/sirupsen/logrus"
)

// execWatcher watches a Execution instance for completion.
type execWatcher struct {
	// c is the controller
	c *Controller
	// cancel is the context cancel func
	cancel context.CancelFunc
	// objKey is the object key from execState
	objKey string
	// execState is the previous state to compare against
	execState *forge_pass.ExecState
}

// startExecWatcher launches a new execution instance watcher.
//
// expects caller to have locked c.mtx
func (c *Controller) startExecWatcher(
	ctx context.Context,
	execState *forge_pass.ExecState,
) *execWatcher {
	subCtx, subCtxCancel := context.WithCancel(ctx)
	objKey := execState.GetObjectKey()
	exc := &execWatcher{
		c:         c,
		cancel:    subCtxCancel,
		objKey:    objKey,
		execState: execState,
	}
	if v := c.execWatchers[objKey]; v != nil {
		v.cancel()
	}
	c.execWatchers[objKey] = exc
	go exc.execute(subCtx)
	return exc
}

// execute executes the Execution watcher.
func (e *execWatcher) execute(ctx context.Context) {
	defer e.cancel()

	execObjKey := e.objKey
	e.c.le.
		WithField("exec-object-key", execObjKey).
		WithField("exec-state", e.execState.GetExecutionState().String()).
		Debug("watching execution object for changes")
	loop, _, ws := world_control.NewBusWatchLoop(
		ctx,
		e.c.le,
		e.c.bus,
		e.c.conf.GetEngineId(),
		false,
		execObjKey,
		e.processState,
	)
	if err := loop.Execute(ctx, ws); err != context.Canceled && err != nil {
		e.c.le.WithError(err).Warn("exec watcher exited with error")
	}

	e.c.mtx.Lock()
	if v := e.c.execWatchers[execObjKey]; v == e {
		delete(e.c.execWatchers, execObjKey)
	}
	e.c.mtx.Unlock()
}

// processState implements the state watcher loop.
func (e *execWatcher) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	// objKey := e.execState.GetObjectKey()
	if obj == nil {
		le.Debug("object does not exist")
		return true, nil
	}

	// unmarshal Execution state + build read cursor
	var exState *forge_execution.Execution
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		exState, berr = forge_execution.UnmarshalExecution(ctx, bcs)
		return berr
	})
	if err != nil {
		return false, err
	}

	// check if the execution state matches the ExecState
	if e.execState.MatchesExecution(exState) {
		// matches, continue to watch
		return true, nil
	}

	// does not match: stop this watcher & notify controller
	e.c.triggerSyncExecStates()
	return false, nil
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = ((*execWatcher)(nil)).processState
