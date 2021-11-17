package pass_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	forge_pass "github.com/aperturerobotics/forge/pass"
	pass_transaction "github.com/aperturerobotics/forge/pass/tx"
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
	// peerID is the parsed peer id
	// may be empty
	peerID peer.ID
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
		peerID: peerID,
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
func (c *Controller) Execute(ctx context.Context) error {
	loop, _ := world_control.NewBusObjectLoop(
		ctx,
		c.le,
		c.bus,
		c.conf.GetEngineId(),
		true,
		c.conf.GetObjectKey(),
		c.ProcessState,
	)
	return loop.Execute(ctx)
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
	objRef, objRev, err := obj.GetRootRef()
	if err != nil {
		if err == world.ErrObjectNotFound {
			return true, nil
		}
		return false, err
	}
	le.Debugf("processing object at rev %v", objRev)

	// unmarshal Pass state + build read cursor
	var exState *forge_pass.Pass
	_, err = world.AccessObject(ctx, ws.AccessWorldState, objRef, func(bcs *block.Cursor) error {
		var berr error
		exState, berr = forge_pass.UnmarshalPass(bcs)
		return berr
	})
	if err != nil {
		return false, err
	}

	// check if completed
	currState := exState.GetPassState()
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

	if currState == forge_pass.State_PassState_CHECKING {
		le.Debug("checking pass execution output")

		// COMPLETE w/ success=true
		txd := pass_transaction.NewTxComplete(forge_value.NewResultWithSuccess())
		_, _, err = ws.ApplyWorldOp(txd, peerID)
		return true, err
	}

	// promote pending -> running
	replicas := c.conf.GetReplicas()
	if replicas == 0 {
		replicas = 1
	}
	if currState == forge_pass.State_PassState_PENDING {
		if replicas == 1 {
			le.Debug("starting pass")
		} else {
			le.Debugf("starting pass with %d replicas", replicas)
		}
		var execSpecs []*pass_transaction.ExecSpec
		if c.conf.GetAssignSelf() {
			execSpecs = []*pass_transaction.ExecSpec{{
				PeerId: peerID.Pretty(),
			}}
		}
		txd := pass_transaction.NewTxStart(replicas, execSpecs)
		if err != nil {
			return false, err
		}
		// the control loop will see the change & run ProcessState again
		_, _, err = ws.ApplyWorldOp(txd, peerID)
		return true, err
	}

	if currState == forge_pass.State_PassState_RUNNING {
		le.Debug("waiting for pass executions to complete")
		// TODO
		txd := pass_transaction.NewTxExecComplete()
		_, _, err = ws.ApplyWorldOp(txd, peerID)
		return true, err
	}

	// unhandled state
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

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
