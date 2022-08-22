package unixfs_mount_fuse

import (
	bfuse "bazil.org/fuse"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/unixfs/fuse"
	unixfs_mount "github.com/aperturerobotics/hydra/unixfs/mount"
	"github.com/aperturerobotics/hydra/util/checkerrs"
	"github.com/sirupsen/logrus"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
// Example: controllerbus/example/boilerplate/1
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return c.EqualVT(ot)
}

// Validate validates the config.
func (c *Config) Validate() error {
	// validate mount path?
	return nil
}

// BuildUnixFSMountController constructs the unixfs_mount.MountController.
// The mount controller should not actually mount until Execute is called.
// If err == nil the MountController should not be nil.
func (c *Config) BuildUnixFSMountController(b bus.Bus, le *logrus.Entry) (unixfs_mount.MountController, error) {
	return NewController(b, le, c), nil
}

// SetMountPath configures the destination path to mount to.
func (c *Config) SetMountPath(npath string) {
	if c != nil {
		c.MountPath = npath
	}
}

// BuildMountOptions builds the fuse mount options.
func (c *Config) BuildFuseMountOptions() []fuse.MountOption {
	mountOpts := []fuse.MountOption{
		bfuse.DefaultPermissions(),
		bfuse.AllowNonEmptyMount(),
	}
	if c.GetAllowOther() {
		mountOpts = append(mountOpts, bfuse.AllowOther())
	}
	if c.GetAllowDev() {
		mountOpts = append(mountOpts, bfuse.AllowDev())
	}
	if c.GetAllowSuid() {
		mountOpts = append(mountOpts, bfuse.AllowSUID())
	}
	return mountOpts
}

// ApplyVolumeMountAttributes applies the CSI volume mount attributes.
// These are extra arguments for the config.
// For example: fuse: allow_other "true" -> enable allow_other.
// The config can optionally ignore attributes that are unknown, or return an error.
func (c *Config) ApplyVolumeMountAttributes(attrs map[string]string) error {
	// security risk?
	// &c.AllowDev, "allow_dev"
	// &c.AllowSuid, "allow_suid"

	return checkerrs.AnyErrors(
		unixfs_mount.ApplyBoolVolumeAttribute(attrs, &c.Verbose, "verbose"),
		unixfs_mount.ApplyBoolVolumeAttribute(attrs, &c.AllowOther, "allow_other"),
	)
}

// _ is a type assertion
var _ unixfs_mount.MountControllerConfig = ((*Config)(nil))
