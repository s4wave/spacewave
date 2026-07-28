package pass_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	world_control "github.com/s4wave/spacewave/db/world/control"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	pass_transaction "github.com/s4wave/spacewave/forge/pass/tx"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/pass"

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
	peerID peer.ID
	// peerIDStr is the string peer id
	peerIDStr string

	// watchExecStatesCh is pushed with the list of exec states to watch.
	// if any of the exec states change, calls UpdateExecStates.
	// if nil is pushed (empty list), shuts down all routines.
	watchExecStatesCh chan []*forge_pass.ExecState
	// syncExecutionsCh is pushed to trigger syncing executions to the pass.
	syncExecutionsCh chan struct{}

	// execWatchers owns the current running set of execution state watchers.
	execWatchers *keyed.Keyed[execWatcherKey, *execWatcher]
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	peerID, _ := conf.ParsePeerID()
	c := &Controller{
		le:        le,
		bus:       bus,
		conf:      conf,
		objKey:    conf.GetObjectKey(),
		peerID:    peerID,
		peerIDStr: peerID.String(),

		watchExecStatesCh: make(chan []*forge_pass.ExecState, 1),
		syncExecutionsCh:  make(chan struct{}, 1),
	}
	c.execWatchers = keyed.NewKeyedWithLogger(c.newExecWatcher, le.WithField("tracker", "exec-watcher"))
	return c
}

// StartControllerWithConfig starts a controller with a config.
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
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"pass controller",
	)
}

// Execute executes the controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()
	c.execWatchers.SetContext(ctx, true)
	defer c.execWatchers.ClearContext()

	errCh := make(chan error, 2)
	loop, busEngine, ws := world_control.NewBusWatchLoop(
		ctx,
		c.le,
		c.bus,
		c.conf.GetEngineId(),
		true,
		c.objKey,
		c.ProcessState,
	)
	go func() {
		errCh <- loop.Execute(ctx, ws)
	}()

	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-errCh:
			if ctx.Err() != nil && errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			return err
		case watchStates := <-c.watchExecStatesCh:
			if err := c.syncWatchExecStates(ctx, watchStates); err != nil {
				return err
			}
		case <-c.syncExecutionsCh:
			// submit transaction to synchronize executions
			c.le.Debug("updating pass execution state snapshots")
			wtx, err := busEngine.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			txd := pass_transaction.NewTxUpdateExecStates(c.objKey)
			_, _, err = wtx.ApplyWorldOp(ctx, txd, c.peerID)
			if err != nil {
				wtx.Discard()
			} else {
				err = wtx.Commit(ctx)
			}
			if err != nil && (ctx.Err() == nil || !errors.Is(err, context.Canceled)) {
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
	watchers := make(map[string]execWatcherKey, len(execStates))
	for _, state := range execStates {
		if state.GetExecutionState() == forge_execution.State_ExecutionState_COMPLETE {
			continue
		}

		watchers[state.GetObjectKey()] = newExecWatcherKey(state)
	}

	keys := make([]execWatcherKey, 0, len(watchers))
	for _, key := range watchers {
		keys = append(keys, key)
	}
	c.execWatchers.SyncKeys(keys, true)

	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	c.execWatchers.ClearContext()
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
		case <-c.watchExecStatesCh:
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
