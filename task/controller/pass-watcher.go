package task_controller

import (
	"context"

	forge_pass "github.com/aperturerobotics/forge/pass"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/sirupsen/logrus"
)

// passWatcher watches a Pass instance for completion.
type passWatcher struct {
	// c is the controller
	c *Controller
	// cancel is the context cancel func
	cancel context.CancelFunc
	// state is the previous state to compare against
	state *passState
}

// startPassWatcher launches a new pass instance watcher.
//
// expects caller to have locked c.mtx
// stops existing worker and returns nil if state == nil
func (c *Controller) startPassWatcher(
	ctx context.Context,
	state *passState,
) *passWatcher {
	subCtx, subCtxCancel := context.WithCancel(ctx)
	if v := c.passWatcher; v != nil {
		v.cancel()
	}
	if state == nil {
		return nil
	}
	exc := &passWatcher{
		c:      c,
		cancel: subCtxCancel,
		state:  state,
	}
	c.passWatcher = exc
	go exc.execute(subCtx)
	return exc
}

// execute executes the Pass watcher.
func (e *passWatcher) execute(ctx context.Context) {
	defer e.cancel()

	passObjKey := e.state.objKey
	passState := e.state.pass.GetPassState()
	e.c.le.
		WithField("pass-object-key", passObjKey).
		WithField("pass-state", passState.String()).
		Debug("task: watching pass for changes")
	loop, _ := world_control.NewBusObjectLoop(
		ctx,
		e.c.le,
		e.c.bus,
		e.c.conf.GetEngineId(),
		false,
		passObjKey,
		e.processState,
	)
	if err := loop.Execute(ctx); err != context.Canceled && err != nil {
		e.c.le.WithError(err).Warn("pass watcher exited with error")
	}

	e.c.mtx.Lock()
	if v := e.c.passWatcher; v == e {
		e.c.passWatcher = nil
	}
	e.c.mtx.Unlock()
}

// processState implements the state watcher loop.
func (e *passWatcher) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	objKey := e.state.objKey
	if obj == nil {
		le.Debug("object does not exist")
		return true, nil
	}

	// unmarshal Pass state + build read cursor
	var passState *forge_pass.Pass
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		passState, berr = forge_pass.UnmarshalPass(bcs)
		return berr
	})
	if err != nil {
		return false, err
	}

	nextState := newPassState(objKey, passState)
	if !nextState.checkChanged(e.state) {
		// matches, continue to watch
		return true, nil
	}

	// does not match: stop this watcher & notify controller
	e.c.triggerSyncPassState(nextState)
	return false, nil
}

// _ is a type assertion
var _ world_control.ObjectLoopHandler = ((*passWatcher)(nil)).processState
