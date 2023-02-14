package unixfs_mount_checkout

import (
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_mount "github.com/aperturerobotics/hydra/unixfs/mount"
	unixfs_sync "github.com/aperturerobotics/hydra/unixfs/sync"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the controller.
const ControllerID = "hydra/unixfs/mount/checkout"

// Version is the version of the implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the mount controller.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// mountedCtr is set when the fs has been mounted
	mountedCtr *ccontainer.CContainer[bool]
	// handle is the fs handle
	handle *unixfs.FSHandle
}

// NewController constructs a new forwarding controller.
func NewController(
	bus bus.Bus,
	le *logrus.Entry,
	conf *Config,
) *Controller {
	return &Controller{
		bus:        bus,
		le:         le,
		conf:       conf,
		mountedCtr: ccontainer.NewCContainer(false),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"unixfs fuse mount",
	)
}

// InitUnixFSMountController initializes the UnixFS mount controller.
// This is called before Execute().
// Any error returned cancels execution of the controller.
func (c *Controller) InitUnixFSMountController(
	ctx context.Context,
	handle *unixfs.FSHandle,
) error {
	c.handle = handle
	return c.conf.Validate()
}

// WaitUnixFSMounted waits for the FS to be mounted or ctx canceled.
// Returns nil when the FS is mounted.
func (c *Controller) WaitUnixFSMounted(ctx context.Context) error {
	_, err := c.mountedCtr.WaitValue(ctx, nil)
	return err
}

// Execute executes the forwarding controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	mountPath := c.conf.GetMountPath()
	// mountVerbose := c.conf.GetVerbose()
	// mountOpts := c.conf.BuildFuseMountOptions()

	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return err
	}

	// checkout the files to the path.
	le := c.le.WithField("mount-path", mountPath)
	le.Debug("checking out files to mount path")
	skipPathPrefixes := c.conf.GetSkipPathPrefixes()
	if err := unixfs_sync.Sync(ctx, mountPath, c.handle, unixfs_sync.DeleteMode_DeleteMode_DURING, skipPathPrefixes); err != nil {
		return err
	}

	le.Debug("done checking out files to mount path")
	c.mountedCtr.SetValue(true)
	<-ctx.Done()
	c.mountedCtr.SetValue(false)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	if c.handle != nil {
		c.handle.Release()
	}
	return nil
}

// _ is a type assertion
var _ unixfs_mount.MountController = ((*Controller)(nil))
