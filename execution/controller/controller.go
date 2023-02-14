package execution_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	forge_execution "github.com/aperturerobotics/forge/execution"
	execution_transaction "github.com/aperturerobotics/forge/execution/tx"
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
const ControllerID = "forge/execution"

// Controller implements the Execution controller.
// An Execution is an attempt to process a given Target.
// Usually constructed & managed by the Pass controller.
// Spawns "exec" controllers on the provided bus.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the execution controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// uniqueID is the derived unique id
	uniqueID string
	// peerID is the parsed peer id
	peerID peer.ID
	// objLoop is the object tracking loop
	objLoop *world_control.ObjectLoop
}

// NewController constructs a new Execution controller.
// Note: exec.controller instances will be run on the given bus.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	peerID, _ := conf.ParsePeerID()
	uniqueID := conf.BuildUniqueID()
	c := &Controller{
		le:       le,
		bus:      bus,
		conf:     conf,
		uniqueID: uniqueID,
		peerID:   peerID,
	}
	c.objLoop = world_control.NewObjectLoop(
		le.WithField("control-loop", "execution"),
		conf.GetObjectKey(),
		c.ProcessState,
	)
	return c
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
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"execution controller",
	)
}

// Execute executes the controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	return world_control.ExecuteBusObjectLoop(
		ctx,
		c.bus,
		c.conf.GetEngineId(),
		true,
		c.objLoop,
	)
}

// ProcessState implements the state reconciliation loop.
func (c *Controller) ProcessState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	if obj == nil {
		le.Debug("object does not exist, waiting")
		return true, nil
	}

	// get latest root ref
	objRef, _, err := obj.GetRootRef()
	if err != nil {
		if err == world.ErrObjectNotFound {
			return true, nil
		}
		return false, err
	}

	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// unmarshal Execution state + build read cursor
	var exState *forge_execution.Execution
	_, err = world.AccessObject(ctx, ws.AccessWorldState, objRef, func(bcs *block.Cursor) error {
		var berr error
		exState, berr = forge_execution.UnmarshalExecution(bcs)
		return berr
	})
	if err != nil {
		return false, err
	}

	// check execution state
	if err := exState.Validate(); err != nil {
		return false, errors.Wrap(err, "initial state is invalid")
	}

	// locally specified peer id
	peerID := c.peerID
	if len(peerID) == 0 {
		// use the peer ID specified on the state
		peerID, err = exState.ParsePeerID()
		if err != nil {
			return true, errors.Wrap(err, "parse peer id on execution state")
		}
	}

	// check if completed
	currState := exState.GetExecutionState()
	if currState == forge_execution.State_ExecutionState_COMPLETE {
		le.Debug("execution is marked as complete")
		return false, nil
	}

	// check peer id matches if set
	if err := exState.CheckPeerID(peerID); err != nil {
		return true, err
	}

	// lookup the peer on the bus
	exPeer, _, peerRef, err := peer.GetPeerWithID(ctx, c.bus, peerID, false, nil)
	if err != nil {
		return false, err
	}
	defer peerRef.Release()
	_ = exPeer

	// promote pending -> running
	if currState == forge_execution.State_ExecutionState_PENDING {
		le.Debugf(
			"marking execution as running with peer id: %s",
			peerID.Pretty(),
		)
		txd := execution_transaction.NewTxStart(peerID)
		if err != nil {
			return false, err
		}
		_, _, err = obj.ApplyObjectOp(txd, peerID)
		if err != nil {
			return false, err
		}
		// the control loop will see the change & run ProcessState again
		return true, nil
	}

	// check if running, otherwise, this is some unknown state
	if currState != forge_execution.State_ExecutionState_RUNNING {
		return true, errors.Wrapf(
			forge_value.ErrUnknownState,
			"%s", currState.String(),
		)
	}

	// process the exec portion of the target
	// note: if an error occurs in exec controller,
	// processExec marks the execution as complete w/ the error and returns nil.
	var tgt *forge_target.Target
	_, err = world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs = bcs.Detach(true)
		bcs.ClearAllRefs()
		bcs.SetRefAtCursor(exState.GetTargetRef(), true)

		var berr error
		tgt, berr = forge_target.UnmarshalTarget(bcs)
		return berr
	})
	if err != nil {
		return true, errors.Wrap(err, "lookup target configuration")
	}

	err = c.processExec(subCtx, tgt, ws, exState)
	if err == context.Canceled {
		return false, err
	}

	// mark the execution as complete w/o error
	var res *forge_value.Result
	if err != nil {
		le.WithError(err).Warn("marking execution as failed w/ error")
		res = forge_value.NewResultWithError(err)
	} else {
		le.Info("marking execution as complete")
		res = forge_value.NewResultWithSuccess()
	}

	// COMPLETE w/ success=true
	txd := execution_transaction.NewTxComplete(res)
	_, _, err = obj.ApplyObjectOp(txd, c.peerID)
	return false, err // done
}

// _ is a type assertion
var _ world_control.ObjectLoopHandler = ((*Controller)(nil)).ProcessState

// CheckExecControllerConfig checks if the controller config is OK to execute.
func (c *Controller) CheckExecControllerConfig(ctx context.Context, conf config.Config) error {
	// no-op - allow all
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
