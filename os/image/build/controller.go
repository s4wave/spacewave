// Package os_image_build builds a Spacewave OS image in the Space as a Forge
// Step.
package os_image_build

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	forge_lib_util_inputfs "github.com/s4wave/spacewave/forge/lib/util/inputfs"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/os/boot"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "os/image/build"

// InputBoot names the input whose init file becomes the image's workload.
const InputBoot = "boot"

// InputBusybox names the input whose busybox file supplies the image's tools.
const InputBusybox = "busybox"

// OutputSystem names the output holding the image's UnixFS tree in the Space.
const OutputSystem = "system"

// OutputImage names the output holding the image store, a tree with
// image.s4wave.
const OutputImage = "image"

// Controller builds one image from its inputs.
type Controller struct {
	// le is the log entry
	le *logrus.Entry
	// conf is the configuration
	conf *Config
	// inputVals holds the boot and busybox inputs.
	inputVals forge_target.InputMap
	// handle contains the controller handle
	handle forge_target.ExecControllerHandle
}

// NewController constructs a new image build controller.
func NewController(le *logrus.Entry, conf *Config) *Controller {
	return &Controller{le: le, conf: conf}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "OS image build controller")
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

// Execute writes the image tree into the Space under the World's transform,
// exports its blocks into an image store naming it as the built image, and
// outputs both. The store is a second copy of the tree's blocks, kept until
// the rehearsal and install read the tree from the Space.
func (c *Controller) Execute(ctx context.Context) error {
	// Hold the exported store in a scratch directory.
	dir, err := os.MkdirTemp("", "os-image-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir) //nolint:errcheck
	store := filepath.Join(dir, OutputImage)

	// Write the tree and export it within one storage access, so the export
	// reads the blocks the write staged.
	var image *bucket.ObjectRef
	err = c.handle.AccessStorage(ctx, nil, func(cs *bucket_lookup.Cursor) error {
		// Write the tree under the World's transform.
		var err error
		image, err = c.writeImage(ctx, cs)
		if err != nil {
			return err
		}

		// Export the tree's blocks, still encoded, into the image store.
		copied, err := boot.ExportImage(ctx, c.le, store, cs.GetBlockStore(), image)
		if err != nil {
			return errors.Wrap(err, "export image")
		}
		c.le.Infof("built image %s; exported %d blocks", image.GetRootRef().MarshalString(), copied)
		return nil
	})
	if err != nil {
		return err
	}

	// Store the image store as a tree and output it beside the tree.
	ref, err := forge_lib_util_inputfs.StoreDir(ctx, c.handle, store)
	if err != nil {
		return err
	}
	return c.handle.SetOutputs(ctx, forge_value.ValueSlice{
		forge_value.NewValueWithBlockRef(OutputSystem, image.GetRootRef()),
		forge_value.NewValueWithBlockRef(OutputImage, ref),
	}, true)
}

// writeImage writes the image from the boot input's init and the busybox
// input's busybox through cs, streaming both files from the Space.
func (c *Controller) writeImage(ctx context.Context, cs *bucket_lookup.Cursor) (*bucket.ObjectRef, error) {
	// Open each input file.
	executable, closeExecutable, err := c.openInputFile(ctx, cs, InputBoot, "init")
	if err != nil {
		return nil, err
	}
	defer closeExecutable()
	busybox, closeBusybox, err := c.openInputFile(ctx, cs, InputBusybox, "busybox")
	if err != nil {
		return nil, err
	}
	defer closeBusybox()

	// Write the tree under the World's transform.
	return boot.WriteImage(ctx, cs, executable, busybox, nil)
}

// openInputFile opens the file name in the named input's tree for reading,
// valid within the storage access that produced cs, with a function that
// closes it.
func (c *Controller) openInputFile(ctx context.Context, cs *bucket_lookup.Cursor, input, name string) (io.Reader, func(), error) {
	// Open the input's tree.
	handle, err := forge_lib_util_inputfs.Open(ctx, cs, c.inputVals, input)
	if err != nil {
		return nil, nil, err
	}

	// Open the file in it.
	file, err := unixfs_iofs.NewFS(ctx, handle).Open(name)
	if err != nil {
		handle.Release()
		return nil, nil, errors.Wrapf(err, "input %q", input)
	}
	return file, func() {
		_ = file.Close()
		handle.Release()
	}, nil
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
