package volume_rpc_server

import (
	"regexp"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a config with a volume id list.
func NewConfig(serviceID string, volumeIdList []string) *Config {
	return &Config{ServiceId: serviceID, VolumeIdList: volumeIdList}
}

// NewConfigWithRe constructs a config with a volume id regex.
func NewConfigWithRe(serviceID, volumeIdRe string) *Config {
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
	if len(c.GetVolumeIdList()) == 0 && len(c.GetVolumeIdRe()) == 0 {
		return errors.New("volume id regex or volume id list is set")
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
