package unixfs_mount_fuse

import (
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/hydra/unixfs/fuse"
	unixfs_mount "github.com/aperturerobotics/hydra/unixfs/mount"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the controller.
const ControllerID = "hydra/unixfs/mount/fuse/1"

// Version is the version of the implementation.
var Version = semver.MustParse("0.0.1")

// Controller implements the fuse mount controller.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
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
		bus:  bus,
		le:   le,
		conf: conf,
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

// Execute executes the forwarding controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	mountPath := c.conf.GetMountPath()
	mountVerbose := c.conf.GetVerbose()
	mountOpts := c.conf.BuildFuseMountOptions()

	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return err
	}

	le := c.le.WithField("mount-path", mountPath)
	rfs, err := fuse.Mount(ctx, le, mountPath, c.handle, mountVerbose, mountOpts)
	if err != nil {
		return err
	}
	defer rfs.Close()
	defer func() {
		le.Info("unmounting UnixFS FUSE mount")
		if err := fuse.Unmount(mountPath); err != nil {
			le.WithError(err).Error("unable to unmount FUSE fs")
		}
		if err := os.Remove(mountPath); err != nil {
			le.WithError(err).Error("unable to remove FUSE fs root")
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- rfs.Serve()
	}()
	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errCh:
		return err
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) (directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ unixfs_mount.MountController = ((*Controller)(nil))
