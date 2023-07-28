package web_pkg_fs_controller

import (
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	"github.com/aperturerobotics/controllerbus/config"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/pkg/errors"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(unixFsID, unixFsPrefix string, notFoundIfIdle bool, webPkgIdList []string) *Config {
	return &Config{
		UnixfsId:       unixFsID,
		UnixfsPrefix:   unixFsPrefix,
		NotFoundIfIdle: notFoundIfIdle,
		WebPkgIdList:   webPkgIdList,
	}
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetUnixfsId() == "" {
		return unixfs_errors.ErrEmptyUnixFsId
	}
	for i, webPkgID := range c.GetWebPkgIdList() {
		if err := web_pkg.ValidateWebPkgId(webPkgID); err != nil {
			return errors.Wrapf(err, "web_pkg_id_list[%d]", i)
		}
	}
	return nil
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return ot.EqualVT(c)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
