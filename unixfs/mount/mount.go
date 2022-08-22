package unixfs_mount

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// MountController is a controller that manages mounting UnixFS cursors to
// locations on the host filesystem.
type MountController interface {
	// Controller indicates this is a controller-bus controller.
	controller.Controller
	// InitUnixFSMountController initializes the UnixFS mount controller.
	// This is called before Execute().
	// Any error returned cancels execution of the controller.
	InitUnixFSMountController(
		ctx context.Context,
		handle *unixfs.FSHandle,
	) error
}

// MountControllerConfig is a configuration for a MountController.
type MountControllerConfig interface {
	// Config indicates this is a controller-bus config.
	config.Config
	// BuildUnixFSMountController constructs the unixfs_mount.MountController.
	// The mount controller should not actually mount until Execute is called.
	// If err == nil the MountController should not be nil.
	BuildUnixFSMountController(b bus.Bus, le *logrus.Entry) (MountController, error)
	// SetMountPath configures the destination path to mount to.
	SetMountPath(npath string)
	// ApplyVolumeMountAttributes applies the CSI volume mount attributes.
	// These are extra arguments for the config.
	// For example: fuse: allow_other "true" -> enable allow_other.
	// The config can optionally ignore attributes that are unknown, or return an error.
	ApplyVolumeMountAttributes(attributes map[string]string) error
}

// ResolveMountControllerConfig resolves a configset.ControllerConfig to a MountControllerConfig.
func ResolveMountControllerConfig(
	ctx context.Context,
	b bus.Bus,
	ctrlConf *configset_proto.ControllerConfig,
) (MountControllerConfig, error) {
	cc, err := ctrlConf.Resolve(ctx, b)
	if err != nil {
		return nil, err
	}
	if cc == nil {
		return nil, errors.Errorf("unable to resolve config: %s", ctrlConf.GetId())
	}
	mountCtrlConf, ok := cc.(MountControllerConfig)
	if !ok {
		return nil, errors.Errorf("must implement MountControllerConfig: %s", ctrlConf.GetId())
	}
	return mountCtrlConf, nil
}

// DefaultMountControllerConfig is the default MountController to use.
var DefaultMountControllerConfig = &configset_proto.ControllerConfig{
	Id: "hydra/unixfs/mount/fuse/1",
}

// ExecuteMountControllerWithConfig executes a MountController by resolving the
// configuration and executing the resulting controller.
//
// buildFSHandle is called just before executing the MountController and must not be nil.
// if ctrlConf is empty, uses the default mount controller (FUSE).
func ExecuteMountControllerWithConfig(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	ctrlConf *configset_proto.ControllerConfig,
	mountAttributes map[string]string,
	buildFSHandle func(ctx context.Context) (*unixfs.FSHandle, error),
	mountPath string,
) error {
	if ctrlConf.GetId() == "" {
		ctrlConf = DefaultMountControllerConfig.CloneVT()
	}

	// resolve the controller config to the MountControllerConfig
	mountCtrlConf, err := ResolveMountControllerConfig(ctx, b, ctrlConf)
	if err != nil {
		return err
	}

	// apply path
	mountCtrlConf.SetMountPath(mountPath)

	// apply attributes
	if len(mountAttributes) != 0 {
		if err := mountCtrlConf.ApplyVolumeMountAttributes(mountAttributes); err != nil {
			return err
		}
	}

	// construct the MountController
	mountCtrl, err := mountCtrlConf.BuildUnixFSMountController(b, le)
	if err != nil {
		return err
	}
	defer func() {
		_ = mountCtrl.Close()
	}()

	// build the FS Handle
	fsHandle, err := buildFSHandle(ctx)
	if err != nil {
		return err
	}

	// init the controller
	err = mountCtrl.InitUnixFSMountController(ctx, fsHandle)
	if err != nil {
		return err
	}

	// execute the controller
	return b.ExecuteController(ctx, mountCtrl)
}
