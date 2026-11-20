// Package camera serves the Device's camera track to authorized account peers.
package camera

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb"
	"github.com/s4wave/spacewave/net/protocol"
)

// ControlProtocolID identifies the camera subscription stream.
const ControlProtocolID = protocol.ID("alpha/camera/control/1")

// ControllerID identifies the camera controller.
const ControllerID = "spacewave/device/camera"

// ConfigID identifies the camera controller configuration.
const ConfigID = ControllerID

// Config selects the built-in camera test pattern.
type Config struct {
	// Empty supplies the protobuf representation for the fieldless config.
	emptypb.Empty
}

// Validate accepts the fieldless camera configuration.
func (c *Config) Validate() error {
	return nil
}

// GetConfigID returns the camera controller's configuration ID.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig reports whether other selects the same camera configuration.
func (c *Config) EqualsConfig(other config.Config) bool {
	_, ok := other.(*Config)
	return ok
}

// _ is a type assertion.
var _ config.Config = (*Config)(nil)
