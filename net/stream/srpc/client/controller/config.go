package stream_srpc_client_controller

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/net/protocol"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// GetConfigID returns the unique string for this configuration type.
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

// Validate checks the config.
func (c *Config) Validate() error {
	var pid protocol.ID = protocol.ID(c.GetProtocolId()) //nolint:staticcheck
	if err := pid.Validate(); err != nil {
		return protocol.ErrEmptyProtocolID
	}
	if err := c.GetClient().Validate(); err != nil {
		return err
	}
	return nil
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
