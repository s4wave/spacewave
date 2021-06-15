package execution_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	forge_execution "github.com/aperturerobotics/forge/execution"
	execution_transaction "github.com/aperturerobotics/forge/execution/transaction"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/execution/1"

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
	// handler is the controller handler.
	// typically implemented by the Pass controller
	handler Handler
}

// NewController constructs a new Execution controller.
// Note: exec.controller instances will be run on the given bus.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
	handler Handler,
) *Controller {
	return &Controller{
		le:      le,
		bus:     bus,
		conf:    conf,
		handler: handler,
	}
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
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// parse peer ID for execution
	peerID, err := c.conf.ParsePeerID()
	if err != nil {
		return err
	}
	le := c.le

	// lookup the peer on the bus
	exPeer, peerRef, err := peer.GetPeerWithID(ctx, c.bus, peerID)
	if err != nil {
		return err
	}
	defer peerRef.Release()
	_ = exPeer

	var nextRev uint64
	for {
		// get the current execution state
		exState, exStateCs, err := c.handler.WaitExecutionState(nextRev)
		if err != nil {
			return err
		}
		nextRev = exState.GetRev() + 1
		_ = exStateCs

		// check if completed
		currState := exState.GetExecutionState()
		if currState == forge_execution.State_ExecutionState_COMPLETE {
			return nil
		}

		// check peer id matches if set
		if err := exState.CheckPeerID(peerID); err != nil {
			return err
		}

		// promote pending -> running
		if currState == forge_execution.State_ExecutionState_PENDING {
			le.Debugf(
				"marking execution as running with peer id: %s",
				peerID.Pretty(),
			)
			nextRev, err = c.handler.ProcessTransaction(
				// START
				execution_transaction.NewTxStart(peerID),
			)
			if err != nil {
				return err
			}
			continue
		}

		// check if running
		if currState != forge_execution.State_ExecutionState_RUNNING {
			return errors.Wrapf(
				forge_execution.ErrUnknownState,
				"%s", currState.String(),
			)
		}

		// process the exec portion of the target
		// note: if an error occurs in exec controller,
		// processExec marks the execution as complete w/ the error and returns nil.
		tgtConf := c.conf.GetTarget()
		if err := c.processExec(subCtx, tgtConf); err != nil {
			return err
		}

		// mark the execution as complete w/o error
		le.Debug("marking execution as complete")
		nextRev, err = c.handler.ProcessTransaction(
			// COMPLETE w/ success=true
			execution_transaction.NewTxComplete(forge_execution.NewResultWithSuccess()),
		)
		if err != nil {
			return err
		}
		return nil // done
	}
}

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
