package forge_lib_util_wait

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"

	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/lib/util/wait/1"

// Controller implements the wait util controller.
type Controller struct {
	// le is the log entry
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the configuration
	conf *Config
	// inputVals is the input values map
	inputVals forge_target.InputMap
	// handle contains the controller handle
	handle forge_target.ExecControllerHandle
}

// NewController constructs a new wait controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	return &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"wait controller",
	)
}

// InitForgeExecController initializes the Forge execution controller.
func (c *Controller) InitForgeExecController(
	ctx context.Context,
	inputVals forge_target.InputMap,
	handle forge_target.ExecControllerHandle,
) error {
	c.inputVals, c.handle = inputVals, handle
	return c.conf.Validate()
}

// Execute executes the given controller.
func (c *Controller) Execute(ctx context.Context) error {
	// copy all objects from inputs to outputs
	var outps forge_value.ValueSlice
	for inpName, inpValue := range c.inputVals {
		ipv, err := forge_target.InputValueToValue(inpValue)
		if err != nil {
			c.le.Debugf("skipping input that could not be cast to a value: %s", inpName)
			continue
		}
		if ipv == nil {
			c.le.Debugf("skipping input that was empty: %s", inpName)
			continue
		}
		c.le.Debugf("setting output from input: %s -> %s", inpName, ipv.GetValueType().String())
		outps = append(outps, ipv)
	}

	return c.handle.SetOutputs(ctx, outps, true)
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
var _ forge_target.ExecController = ((*Controller)(nil))
