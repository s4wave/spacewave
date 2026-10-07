package forge_lib_util_presence

import (
	"bytes"
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/lib/util/presence"

// Controller sets one of two outputs by whether a host path exists.
type Controller struct {
	// le is the log entry.
	le *logrus.Entry
	// bus is the controller bus.
	bus bus.Bus
	// conf is the configuration.
	conf *Config
	// handle stores and publishes the chosen output.
	handle forge_target.ExecControllerHandle
}

// NewController constructs a new presence controller.
func NewController(le *logrus.Entry, bus bus.Bus, conf *Config) *Controller {
	return &Controller{le: le, bus: bus, conf: conf}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "path presence controller")
}

// InitForgeExecController initializes the Forge execution controller.
func (c *Controller) InitForgeExecController(
	ctx context.Context,
	inputVals forge_target.InputMap,
	handle forge_target.ExecControllerHandle,
) error {
	c.handle = handle
	return c.conf.Validate()
}

// Execute checks the path and publishes exactly one output holding it.
func (c *Controller) Execute(ctx context.Context) error {
	// Choose the output from the path's presence; other errors fail the Pass.
	path := c.conf.GetPath()
	name := c.conf.GetPresentOutput()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		name = c.conf.GetAbsentOutput()
	} else if err != nil {
		return errors.Wrap(err, "stat path")
	}

	// Store the path as a blob, since outputs without a value are dropped.
	seed := forge_value.NewValueWithBlockRef("", nil)
	val, err := forge_target.AccessValue(ctx, c.handle, seed, func(bcs *block.Cursor) error {
		_, err := blob.BuildBlob(ctx, int64(len(path)), bytes.NewReader([]byte(path)), bcs, nil)
		return err
	})
	if err != nil {
		return errors.Wrap(err, "store path")
	}
	val.Name = name
	return c.handle.SetOutputs(ctx, forge_value.ValueSlice{val}, true)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ forge_target.ExecController = (*Controller)(nil)
