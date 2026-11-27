// Package os_release_publish publishes a signed OS image as a release, as a
// Forge Step whose output an object sink writes as an os/release object.
package os_release_publish

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_lib_util_inputfs "github.com/s4wave/spacewave/forge/lib/util/inputfs"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	os_image "github.com/s4wave/spacewave/os/image"
	os_release "github.com/s4wave/spacewave/os/release"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "os/release/publish"

// InputAuthorization identifies the input holding the sign Step's tree with the
// authorization file.
const InputAuthorization = "authorization"

// OutputRelease identifies the output holding the os/release block.
const OutputRelease = "release"

// Controller publishes one authorization as a release.
type Controller struct {
	// conf is the configuration.
	conf *Config
	// inputVals holds the authorization input.
	inputVals forge_target.InputMap
	// handle contains the controller handle.
	handle forge_target.ExecControllerHandle
}

// NewController constructs a new release publish controller.
func NewController(conf *Config) *Controller {
	return &Controller{conf: conf}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "OS release publish controller")
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

// Execute reads the authorization and outputs it as an os/release block in
// the Space.
func (c *Controller) Execute(ctx context.Context) error {
	// Check out the authorization tree and read its file.
	dir, err := os.MkdirTemp("", "os-release-publish-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir) //nolint:errcheck
	if err := forge_lib_util_inputfs.CheckoutAll(ctx, c.handle, c.inputVals, dir, []string{InputAuthorization}); err != nil {
		return err
	}
	signed, err := os_image.ReadAuthorization(filepath.Join(dir, InputAuthorization, os_image.AuthorizationFile))
	if err != nil {
		return errors.Wrapf(err, "input %q", InputAuthorization)
	}

	// Refuse an authorization that references no complete image, then store the
	// release as the output's root block.
	release, err := os_release.NewRelease(signed)
	if err != nil {
		return err
	}
	val, err := forge_target.AccessValue(ctx, c.handle, forge_value.NewValueWithBlockRef("", nil), func(bcs *block.Cursor) error {
		bcs.SetBlock(release, true)
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "store release")
	}
	val.Name = OutputRelease
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
var _ forge_target.ExecController = ((*Controller)(nil))
