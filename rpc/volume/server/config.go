package rpc_volume_server

import (
	"regexp"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/starpc/srpc"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a config.
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

// ParseVolumeIdRe parses the volume id regex field.
// Returns nil if the field was empty.
func (c *Config) ParseVolumeIdRe() (*regexp.Regexp, error) {
	r := c.GetVolumeIdRe()
	if r == "" {
		return nil, nil
	}
	return regexp.Compile(r)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
