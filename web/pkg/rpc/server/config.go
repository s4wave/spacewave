package web_pkg_rpc_server

import (
	"regexp"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a config with a web pkg id list.
func NewConfig(serviceIDPrefix string, webPkgIdList []string) *Config {
	return &Config{ServiceIdPrefix: serviceIDPrefix, WebPkgIdList: webPkgIdList}
}

// NewConfigWithRe constructs a config with a web pkg id regex.
func NewConfigWithRe(serviceIDPrefix, webPkgIdRe string) *Config {
	return &Config{ServiceIdPrefix: serviceIDPrefix, WebPkgIdRe: webPkgIdRe}
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if len(c.GetWebPkgIdList()) == 0 && len(c.GetWebPkgIdRe()) == 0 {
		return errors.New("web pkg id regex or web pkg id list is set")
	}
	if _, err := c.ParseWebPkgIdRe(); err != nil {
		return err
	}
	if _, err := c.ParseReleaseDelay(); err != nil {
		return errors.Wrap(err, "release_delay")
	}
	return nil
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}
	return c.EqualVT(ot)
}

// ParseWebPkgIdRe parses the web pkg id regex field.
// Returns nil if the field was empty.
func (c *Config) ParseWebPkgIdRe() (*regexp.Regexp, error) {
	r := c.GetWebPkgIdRe()
	if r == "" {
		return nil, nil
	}
	return regexp.Compile(r)
}

// ParseReleaseDelay parses the release delay field.
// Applies the default value if the field is empty.
func (c *Config) ParseReleaseDelay() (time.Duration, error) {
	delayStr := c.GetReleaseDelay()
	if delayStr == "" {
		return time.Second, nil
	}
	return time.ParseDuration(delayStr)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
