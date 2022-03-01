package bridge_volume

import (
	"errors"

	"github.com/aperturerobotics/controllerbus/config"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/golang/protobuf/proto"
)

// ConfigID is the identifier for the config type.
const ConfigID = ControllerID

// NewControllerConfig constructs a configset_proto object for cresolve.
func NewControllerConfig(volumeID string) *configset_proto.ControllerConfig {
	conf := &Config{VolumeId: volumeID}
	dat, _ := proto.Marshal(conf)
	return &configset_proto.ControllerConfig{
		Id:     ConfigID,
		Config: dat,
	}
}

// GetConfigID returns the config identifier.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks equality between two configs.
func (c *Config) EqualsConfig(c2 config.Config) bool {
	oc, ok := c2.(*Config)
	if !ok {
		return false
	}

	return proto.Equal(c, oc)
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetVolumeId() == "" {
		return errors.New("volume_id cannot be empty")
	}
	return nil
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
