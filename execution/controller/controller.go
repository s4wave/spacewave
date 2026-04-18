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
	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/aperturerobotics/util/routine"
	"github.com/blang/semver/v4"
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
	// busEngine is the bus world engine handle
	busEngine *world.BusEngine
	// ws is the world state handle for busEngine
	ws world.WorldState
	// objLoop is the object tracking loop
	objLoop *world_control.WatchLoop
	// execRoutine is the execution routine resolving execResult
	// note: value_set and result are set to nil
	execRoutine *routine.StateRoutineContainer[*ExecConfig]
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
	c.busEngine = world.NewBusEngine(nil, bus, conf.GetEngineId())
	c.ws = world.NewEngineWorldState(c.busEngine, true)
	c.objLoop = world_control.NewWatchLoop(
		le.WithField("control-loop", "execution"),
		conf.GetObjectKey(),
		c.ProcessState,
	)
	c.execRoutine = routine.NewStateRoutineContainerWithLogger(
		protobuf_go_lite.CompareEqualVT[*ExecConfig](),
		le.WithField("routine", "execution"),
	)
	c.execRoutine.SetStateRoutine(c.executeWithConfig)
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
	c.execRoutine.SetContext(ctx, true)
	c.busEngine.SetContext(ctx)
	return c.objLoop.Execute(ctx, c.ws)
}

// ProcessState implements the state reconciliation loop.
//
// NOTE: the Execution may be updated by the controller several times during
// execution.
func (c *Controller) ProcessState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	if obj == nil {
		le.Debug("object does not exist, waiting")
		c.execRoutine.SetState(nil)
		return true, nil
	}

	// get latest root ref
	objRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		if err == world.ErrObjectNotFound {
			le.Debug("object does not exist, waiting")
			c.execRoutine.SetState(nil)
			return true, nil
		}
		return false, err
	}

	// unmarshal Execution state + build read cursor
	var exState *forge_execution.Execution
	_, err = world.AccessObject(ctx, ws.AccessWorldState, objRef, func(bcs *block.Cursor) error {
		var berr error
		exState, berr = forge_execution.UnmarshalExecution(ctx, bcs)
		return berr
	})
	if err != nil {
		c.execRoutine.SetState(nil)
		return false, err
	}

	// check execution state
	if err := exState.Validate(); err != nil {
		c.execRoutine.SetState(nil)
		return false, errors.Wrap(err, "initial state is invalid")
	}

	// locally specified peer id
	peerID := c.peerID
	if len(peerID) == 0 {
		// use the peer ID specified on the state
		peerID, err = exState.ParsePeerID()
		if err != nil {
			c.execRoutine.SetState(nil)
			return true, errors.Wrap(err, "parse peer id on execution state")
		}
	}

	// check if completed
	currState := exState.GetExecutionState()
	if currState == forge_execution.State_ExecutionState_COMPLETE {
		le.Debug("execution is marked as complete")
		c.execRoutine.SetState(nil)
		return false, nil
	}

	// check peer id matches if set
	if err := exState.CheckPeerID(peerID); err != nil {
		c.execRoutine.SetState(nil)
		return true, err
	}

	// lookup the peer on the bus (wait for it to exist)
	exPeer, _, peerRef, err := peer.GetPeerWithID(ctx, c.bus, peerID, false, nil)
	if err != nil {
		c.execRoutine.SetState(nil)
		return false, err
	}
	defer peerRef.Release()
	_ = exPeer

	// promote pending -> running
	if currState == forge_execution.State_ExecutionState_PENDING {
		c.execRoutine.SetState(nil)
		le.Debugf(
			"marking execution as running with peer id: %s",
			peerID.String(),
		)
		txd := execution_transaction.NewTxStart(peerID)
		if err != nil {
			return false, err
		}
		_, _, err = obj.ApplyObjectOp(ctx, txd, peerID)
		if err != nil {
			return false, err
		}
		// the control loop will see the change & run ProcessState again
		return true, nil
	}

	// check if running, otherwise, this is some unknown state
	if currState != forge_execution.State_ExecutionState_RUNNING {
		c.execRoutine.SetState(nil)
		return true, errors.Wrapf(
			forge_value.ErrUnknownState,
			"%s", currState.String(),
		)
	}

	// check if equivalent to the current
	execConfigState := exState.CloneVT()
	execConfigState.Result = nil
	execConfigState.LogEntries = nil
	if execConfigState.ValueSet == nil {
		execConfigState.ValueSet = &forge_target.ValueSet{}
	} else {
		execConfigState.ValueSet.Outputs = nil
	}
	prevConfigState := c.execRoutine.GetState()
	if !prevConfigState.GetExecution().EqualVT(execConfigState) {
		var tgt *forge_target.Target
		_, err = world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
			bcs = bcs.Detach(true)
			bcs.ClearAllRefs()
			bcs.SetRefAtCursor(exState.GetTargetRef(), true)

			var berr error
			tgt, berr = forge_target.UnmarshalTarget(ctx, bcs)
			return berr
		})
		if err != nil {
			c.execRoutine.SetState(nil)
			return true, errors.Wrap(err, "lookup target configuration")
		}

		// update the exec configuration
		// note: SetState checks ExecConfig for equality.
		c.execRoutine.SetState(&ExecConfig{
			Execution: execConfigState,
			Target:    tgt,
		})
	}

	return true, nil
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = ((*Controller)(nil)).ProcessState

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
