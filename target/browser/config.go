//go:build js
// +build js

package browser

import (
	"errors"

	"github.com/aperturerobotics/bldr/runtime"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/golang/protobuf/proto"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if v := c.GetRuntimeId(); v == "" {
		return errors.New("runtime id must be set")
	}

	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	return proto.Equal(c, other)
}

// _ is a type assertion
var _ runtime.RuntimeConfig = ((*Config)(nil))
