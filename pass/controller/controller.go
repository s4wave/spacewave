package pass_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	forge_execution "github.com/aperturerobotics/forge/execution"
	forge_pass "github.com/aperturerobotics/forge/pass"
	pass_transaction "github.com/aperturerobotics/forge/pass/tx"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/pass/1"

// Controller implements the Pass controller.
// An Pass is an attempt to process a given Target with Executions.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the execution controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// objKey is the object key (from the config)
	objKey string
	// peerID is the parsed peer id
	// may be empty
	peerID peer.ID

	// watchExecStatesCh is pushed with the list of exec states to watch.
	// if any of the exec states change, calls UpdateExecStates.
	// if nil is pushed (empty list), shuts down all routines.
	watchExecStatesCh chan []*forge_pass.ExecState
	// syncExecutionsCh is pushed to trigger syncing executions to the pass.
	syncExecutionsCh chan struct{}

	// mtx guards below fields
	mtx sync.Mutex
	// execWatchers is the current running set of watchers
	// keyed by object key
	execWatchers map[string]*execWatcher
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	peerID, _ := conf.ParsePeerID()
	return &Controller{
		le:     le,
		bus:    bus,
		conf:   conf,
		objKey: conf.GetObjectKey(),
		peerID: peerID,

		watchExecStatesCh: make(chan []*forge_pass.ExecState, 1),
		execWatchers:      make(map[string]*execWatcher, 1),
		syncExecutionsCh:  make(chan struct{}, 1),
	}
}

// StartControllerWithConfig starts a execution controller with a config.
// Waits for the controller to start.
// Returns a Release function to close the controller when done.
func StartControllerWithConfig(
	ctx context.Context,
	b bus.Bus,
	conf *Config,
) (*Controller, directive.Reference, error) {
	ctrli, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	cl, ok := ctrli.(*Controller)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return cl, ctrlRef, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() controller.Info {
	return controller.Info{
		Id:      ControllerID,
		Version: Version.String(),
	}
}

// Execute executes the controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	errCh := make(chan error, 2)
	loop, busEngine := world_control.NewBusObjectLoop(
		ctx,
		c.le,
		c.bus,
		c.conf.GetEngineId(),
		true,
		c.objKey,
		c.ProcessState,
	)
	go func() {
		errCh <- loop.Execute(ctx)
	}()

	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-errCh:
			return err
		case watchStates := <-c.watchExecStatesCh:
			if err := c.syncWatchExecStates(ctx, watchStates); err != nil {
				return err
			}
		case <-c.syncExecutionsCh:
			// submit transaction to synchronize executions
			c.le.Debug("updating pass execution state snapshots")
			wtx, err := busEngine.NewTransaction(true)
			if err != nil {
				return err
			}
			txd := pass_transaction.NewTxUpdateExecStates(c.objKey)
			_, _, err = wtx.ApplyWorldOp(txd, c.peerID)
			if err != nil {
				wtx.Discard()
			} else {
				err = wtx.Commit(ctx)
			}
			if err != nil && err != context.Canceled {
				c.le.WithError(err).Warn("unable to update execution states")
			}
		}
	}
}

// syncWatchExecStates starts/stop routines to watch execution states.
// called by Execute
func (c *Controller) syncWatchExecStates(ctx context.Context, execStates []*forge_pass.ExecState) error {
	// build map of watchers that should be running
	// skip any executions that are in a terminal state
	watchers := make(map[string]*forge_pass.ExecState, len(execStates))
	for _, state := range execStates {
		if state.GetExecutionState() == forge_execution.State_ExecutionState_COMPLETE {
			continue
		}

		stateObjKey := state.GetObjectKey()
		watchers[stateObjKey] = state
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

	// remove any outdated existing watchers
	for key, exw := range c.execWatchers {
		var rmWatcher bool
		nw := watchers[key]
		if nw != nil {
			rmWatcher = !exw.execState.Equals(nw)
		} else {
			rmWatcher = true
		}
		if rmWatcher {
			exw.cancel()
			delete(c.execWatchers, key)
		} else {
			// watcher exists already
			delete(watchers, key)
		}
	}

	// add any new watchers
	for _, nw := range watchers {
		_ = c.startExecWatcher(ctx, nw)
	}

	return nil
}

// ProcessState implements the state reconciliation loop.
func (c *Controller) ProcessState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	objKey := c.objKey
	if obj == nil {
		le.Debug("object does not exist, waiting")
		return true, nil
	}

	// unmarshal Pass state + build read cursor
	var passState *forge_pass.Pass
	var tgt *forge_target.Target
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		passState, berr = forge_pass.UnmarshalPass(bcs)
		if berr != nil {
			return berr
		}

		tgt, _, berr = passState.FollowTargetRef(bcs)
		return berr
	})
	if err != nil {
		return false, err
	}
	_ = tgt

	// signal to the controller to stop watching for exec states
	currState := passState.GetPassState()
	if currState != forge_pass.State_PassState_RUNNING {
		c.pushWatchExecStates(nil)
	}

	// check if completed
	if currState == forge_pass.State_PassState_COMPLETE {
		le.Debug("pass is marked as complete")
		return false, nil
	}

	// lookup the peer on the bus
	peerID := c.peerID
	exPeer, peerRef, err := peer.GetPeerWithID(ctx, c.bus, peerID)
	if err != nil {
		return false, err
	}
	defer peerRef.Release()
	peerID = exPeer.GetPeerID()

	execStates := passState.GetExecStates()
	if currState == forge_pass.State_PassState_CHECKING {
		le.Debug("TODO check pass execution outputs")

		// asserts that len(execStates) != 0
		if err := passState.Validate(); err != nil {
			// COMPLETE w/ success=false
			txd := pass_transaction.NewTxComplete(objKey, forge_value.NewResultWithError(err))
			_, _, err = ws.ApplyWorldOp(txd, peerID)
			return false, err
		}

		// verify that the outputs look correct
		// currently: we check that the output hashes match.
		exState := execStates[0]

		// build the output set according to the target
		// TODO TODO
		_ = exState

		// COMPLETE w/ success=true
		// this will use the values from the first ExecState
		txd := pass_transaction.NewTxComplete(objKey, forge_value.NewResultWithSuccess())
		_, _, err = ws.ApplyWorldOp(txd, peerID)
		return true, err
	}

	// promote pending -> running
	if currState == forge_pass.State_PassState_PENDING {
		var execSpecs []*pass_transaction.ExecSpec
		if len(execStates)+len(execSpecs) < int(passState.GetReplicas()) {
			if c.conf.GetAssignSelf() {
				execSpecs = []*pass_transaction.ExecSpec{{
					PeerId: peerID.Pretty(),
				}}
			}
		}

		// apply the transaction to start the executions
		// the control loop will see the change & run ProcessState again
		le.Debug("starting pass")
		txd := pass_transaction.NewTxStart(objKey, execSpecs, true)
		_, _, err = ws.ApplyWorldOp(txd, peerID)
		return true, err
	}

	if currState == forge_pass.State_PassState_RUNNING {
		le.Debug("waiting for pass executions to complete")

		// signal to the controller to start / update watchers
		c.pushWatchExecStates(passState.GetExecStates())
		return true, nil
	}

	// unknown state
	return true, errors.Wrapf(
		forge_value.ErrUnknownState,
		"%s", currState.String(),
	)
}

// _ is a type assertion
var _ world_control.ObjectLoopHandler = ((*Controller)(nil)).ProcessState

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
// The context passed is canceled when the directive instance expires.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) (directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// pushWatchExecStates pushes the list of exec states to watch.
func (c *Controller) pushWatchExecStates(states []*forge_pass.ExecState) {
	for {
		select {
		case c.watchExecStatesCh <- states:
			return
		default:
		}
		select {
		case _ = <-c.watchExecStatesCh:
		default:
		}
	}
}

// triggerSyncExecStates triggers re-syncing the execution states.
func (c *Controller) triggerSyncExecStates() {
	select {
	case c.syncExecutionsCh <- struct{}{}:
	default:
	}
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
