package task_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	task_transaction "github.com/aperturerobotics/forge/task/tx"
	"github.com/aperturerobotics/hydra/block"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/task/1"

// Controller implements the Task controller.
// An Task is an attempt to process a given Target with Pass.
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
	// peerIDStr is the parsed peer id string
	peerIDStr string

	// watchPassCh is pushed with the latest Pass state to watch.
	// if nil is pushed, shuts down watcher.
	watchPassCh chan *passState
	// syncPassCh is pushed to trigger syncing Pass to the task.
	syncPassCh chan *passState

	// mtx guards below fields
	mtx sync.Mutex
	// passWatcher is the current watcher for the running Pass.
	passWatcher *passWatcher
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	peerID, _ := conf.ParsePeerID()
	return &Controller{
		le:        le,
		bus:       bus,
		conf:      conf,
		objKey:    conf.GetObjectKey(),
		peerID:    peerID,
		peerIDStr: peerID.Pretty(),

		watchPassCh: make(chan *passState, 1),
		syncPassCh:  make(chan *passState, 1),
	}
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
		case watchState := <-c.watchPassCh:
			c.syncWatchPassStates(ctx, watchState)
		case latestPassState := <-c.syncPassCh:
			// submit transaction to synchronize pass state
			c.le.Debugf(
				"updating task state with pass state: %s",
				latestPassState.pass.GetPassState().String(),
			)
			wtx, err := busEngine.NewTransaction(true)
			if err != nil {
				return err
			}
			txd := task_transaction.NewTxUpdatePassState(c.objKey)
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

// syncWatchPassStates starts/stop routines to watch the latest Pass state.
// called by Execute
func (c *Controller) syncWatchPassStates(ctx context.Context, latestState *passState) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if passWatcher := c.passWatcher; passWatcher != nil {
		if latestState != nil && !c.passWatcher.state.checkChanged(latestState) {
			return
		}
		c.passWatcher.cancel()
		c.passWatcher = nil
	}

	// start watcher if necessary
	if latestState != nil && latestState.pass != nil && latestState.objKey != "" {
		c.startPassWatcher(ctx, latestState)
	}
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
// The context tasked is canceled when the directive instance expires.
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

// pushWatchPassState pushes the latest pass state to watch for changes.
func (c *Controller) pushWatchPassState(state *passState) {
	for {
		select {
		case c.watchPassCh <- state:
			return
		default:
		}
		select {
		case <-c.watchPassCh:
		default:
		}
	}
}

// triggerSyncPassState triggers re-syncing the pass state.
func (c *Controller) triggerSyncPassState(latestState *passState) {
	for {
		select {
		case c.syncPassCh <- latestState:
			return
		default:
		}
		select {
		case <-c.syncPassCh:
		default:
		}
	}
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
