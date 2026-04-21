package volume_rpc_client

import (
	"regexp"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a config.
//
// volumeIdRe is optional.
func NewConfig(serviceID, volumeIdRe string) *Config {
	return &Config{ServiceId: serviceID, VolumeIdRe: volumeIdRe}
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if c.GetServiceId() == "" {
		return srpc.ErrEmptyServiceID
	}
	if _, err := c.ParseVolumeIdRe(); err != nil {
		return err
	}
	if _, err := c.ParseReleaseDelay(); err != nil {
		return errors.Wrap(err, "release_delay")
	}
	return nil
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// ParseVolumeIdRe parses the volume id regex field.
// Returns nil if the field was empty.
func (c *Config) ParseVolumeIdRe() (*regexp.Regexp, error) {
	r := c.GetVolumeIdRe()
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
