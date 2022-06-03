//go:build js
// +build js

package volume_indexeddb

import (
	"errors"

	"github.com/aperturerobotics/controllerbus/config"
	"google.golang.org/protobuf/proto"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if c.GetDatabaseName() == "" {
		return errors.New("database name required")
	}

	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ControllerID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}

	return proto.Equal(c, ot)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
