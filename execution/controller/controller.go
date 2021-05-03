package execution_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver"
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
	tgtConf := c.conf.GetTarget()
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// process the exec portion of the target
	if err := c.processExec(subCtx, tgtConf); err != nil {
		return err
	}

	// done
	return nil
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
	// TODO
	/*
		dir := inst.GetDirective()
		switch d := dir.(type) {
		case boilerplate.Boilerplate:
			return c.resolveBoilerplate(ctx, inst, d)
		}
	*/

	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	// TODO
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
