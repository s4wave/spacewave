// Package os_image_sign signs a rehearsed OS image root as a Forge Step.
package os_image_sign

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_lib_util_inputfs "github.com/s4wave/spacewave/forge/lib/util/inputfs"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/os/boot"
	os_image "github.com/s4wave/spacewave/os/image"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "os/image/sign"

// InputImage names the input holding the image store, a tree with
// image.s4wave, whose built root the Step signs.
const InputImage = "image"

// InputRehearsed names the input holding the rehearsal's receipt, a tree whose
// root file names the image root the rehearsal booted.
const InputRehearsed = "rehearsed"

// OutputAuthorization names the output tree holding the authorization file,
// the os_image.AuthorizationFile that install publishes into the image store.
const OutputAuthorization = "authorization"

// Controller signs one image root with the Worker peer's key.
type Controller struct {
	// le is the log entry
	le *logrus.Entry
	// b is the Worker's bus, which reaches the holder of its peer's key.
	b bus.Bus
	// conf is the configuration
	conf *Config
	// inputVals holds the image and rehearsed inputs.
	inputVals forge_target.InputMap
	// handle contains the controller handle
	handle forge_target.ExecControllerHandle
}

// NewController constructs a new image sign controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) *Controller {
	return &Controller{le: le, b: b, conf: conf}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "OS image sign controller")
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

// Execute signs the selected root of the image input's store as the peer the
// Execution runs as, and outputs the authorization as the tree install
// mounts. The key stays with its holder; only the signature crosses the bus.
func (c *Controller) Execute(ctx context.Context) error {
	// Check out the image store and the rehearsal's receipt.
	dir, err := os.MkdirTemp("", "os-image-sign-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir) //nolint:errcheck
	if err := forge_lib_util_inputfs.CheckoutAll(ctx, c.handle, c.inputVals, dir, []string{InputImage, InputRehearsed}); err != nil {
		return err
	}

	// Read the store's selected root, the root the booting machine opens, and
	// refuse it unless the rehearsal booted that root.
	root, err := boot.ReadImageRoot(ctx, filepath.Join(dir, InputImage))
	if err != nil {
		return errors.Wrapf(err, "input %q", InputImage)
	}
	booted, err := readRehearsedRoot(filepath.Join(dir, InputRehearsed))
	if err != nil {
		return errors.Wrapf(err, "input %q", InputRehearsed)
	}
	if !booted.EqualsRef(root) {
		return errors.Errorf("rehearsal booted %s, not the image's root %s", booted.MarshalString(), root.MarshalString())
	}
	data, err := os_image.MarshalImageRoot(root)
	if err != nil {
		return err
	}

	// Ask the key's holder to sign, and check the answer as a booting machine
	// pinning this peer would.
	signer := c.handle.GetPeerId()
	signed, err := peer.ExSignWithPeer(ctx, c.b, signer, os_image.ImageRootContext, data)
	if err != nil {
		return errors.Wrap(err, "sign image root")
	}
	authorized, _, err := os_image.VerifyImageRoot(signed, []peer.ID{signer})
	if err != nil {
		return err
	}
	if !authorized.EqualsRef(root) {
		return errors.Errorf("signer authorized %s, not %s", authorized.MarshalString(), root.MarshalString())
	}

	// Store the authorization file as the output tree.
	out := filepath.Join(dir, OutputAuthorization)
	if err := os.Mkdir(out, 0o700); err != nil {
		return err
	}
	if err := os_image.WriteAuthorization(filepath.Join(out, os_image.AuthorizationFile), signed); err != nil {
		return err
	}
	ref, err := forge_lib_util_inputfs.StoreDir(ctx, c.handle, out)
	if err != nil {
		return err
	}
	return c.handle.SetOutputs(ctx, forge_value.ValueSlice{
		forge_value.NewValueWithBlockRef(OutputAuthorization, ref),
	}, true)
}

// readRehearsedRoot reads the image root a rehearsal receipt names: the root
// file rehearse.sh writes from the guest's own authorization report.
func readRehearsedRoot(receipt string) (*block.BlockRef, error) {
	data, err := os.ReadFile(filepath.Join(receipt, "root"))
	if err != nil {
		return nil, err
	}
	return block.UnmarshalBlockRefB58(strings.TrimSpace(string(data)))
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
