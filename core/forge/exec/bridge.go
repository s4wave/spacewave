package space_exec

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/sirupsen/logrus"
)

// bridgeVersion is the version for bridge factories and controllers.
var bridgeVersion = semver.MustParse("0.0.1")

// BridgeFactory creates a bus-compatible controller factory from a
// SpaceExecRegistry handler. This adapts space-aware handlers (no bus.Bus
// access) to work within the existing forge execution controller dispatch.
//
// The forge execution controller resolves config IDs through the bus:
//   - LoadConfigConstructorByID finds this factory (via static resolver)
//   - ConstructConfig returns a SpaceExecConfig that stores raw config bytes
//   - LoadFactoryByConfig finds this factory again
//   - Construct creates a bridgeController delegating to the SpaceExecRegistry
//
// This makes all space-exec handlers discoverable through the standard bus
// mechanism, so other plugins can also register handlers by adding their own
// controller factories.
type BridgeFactory struct {
	configID string
	registry *Registry
}

// NewBridgeFactory creates a bridge factory for the given config ID.
func NewBridgeFactory(configID string, registry *Registry) *BridgeFactory {
	return &BridgeFactory{
		configID: configID,
		registry: registry,
	}
}

// GetConfigID returns the config ID this factory handles.
func (f *BridgeFactory) GetConfigID() string {
	return f.configID
}

// ConstructConfig returns a SpaceExecConfig that stores raw config bytes.
// The forge execution controller's Resolve path calls this, then unmarshals
// the target's config data into the returned object.
func (f *BridgeFactory) ConstructConfig() config.Config {
	return NewSpaceExecConfig(f.configID)
}

// Construct creates a bridge controller that delegates to SpaceExecRegistry.
func (f *BridgeFactory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	le := opts.GetLogger()

	// Extract raw config bytes from the config object.
	blk, ok := conf.(block.Block)
	if !ok {
		return nil, errors.New("exec handler config does not implement block.Block")
	}
	configData, err := blk.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal exec handler config")
	}

	return &bridgeController{
		le:         le,
		registry:   f.registry,
		configID:   f.configID,
		configData: configData,
	}, nil
}

// GetVersion returns the bridge factory version.
func (f *BridgeFactory) GetVersion() semver.Version {
	return bridgeVersion
}

// _ is a type assertion
var _ controller.Factory = (*BridgeFactory)(nil)

// bridgeController adapts a SpaceExecRegistry handler to the
// controller.Controller + forge_target.ExecController interface.
// It extracts world state from the input map and delegates execution
// to the registry handler.
type bridgeController struct {
	le         *logrus.Entry
	registry   *Registry
	configID   string
	configData []byte
	handler    Handler
}

// GetControllerInfo returns bridge controller info.
func (c *bridgeController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"space-exec/bridge/"+c.configID,
		bridgeVersion,
		"space exec bridge for "+c.configID,
	)
}

// HandleDirective returns nil (bridge controllers do not handle directives).
func (c *bridgeController) HandleDirective(_ context.Context, _ directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// InitForgeExecController extracts world state from the input map and creates
// the space handler via the registry.
func (c *bridgeController) InitForgeExecController(
	ctx context.Context,
	inputVals forge_target.InputMap,
	handle forge_target.ExecControllerHandle,
) error {
	// Extract world state from the "world" input.
	ws, err := forge_target.InputValueToWorldState(inputVals["world"])
	if err != nil {
		return errors.Wrap(err, "resolve world input")
	}

	handler, err := c.registry.CreateHandler(
		ctx, c.le, ws, handle, inputVals, c.configID, c.configData,
	)
	if err != nil {
		return err
	}
	c.handler = handler
	return nil
}

// Execute runs the space handler.
func (c *bridgeController) Execute(ctx context.Context) error {
	return c.handler.Execute(ctx)
}

// Close releases resources.
func (c *bridgeController) Close() error {
	return nil
}

// _ is a type assertion
var _ forge_target.ExecController = (*bridgeController)(nil)
