// Package camera serves the Device's camera track to authorized account peers.
package camera

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/net/protocol"
)

// ControlProtocolID identifies the camera subscription stream.
const ControlProtocolID = protocol.ID("alpha/camera/control/1")

// ControllerID identifies the camera controller.
const ControllerID = "spacewave/device/camera"

// ConfigID identifies the camera controller configuration.
const ConfigID = ControllerID

// Validate accepts a device path or an empty path for the test pattern.
func (c *Config) Validate() error {
	return nil
}

// GetConfigID returns the camera controller's configuration ID.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig reports whether other selects the same camera configuration.
func (c *Config) EqualsConfig(other config.Config) bool {
	o, ok := other.(*Config)
	return ok && c.EqualVT(o)
}

// _ is a type assertion.
var _ config.Config = (*Config)(nil)
