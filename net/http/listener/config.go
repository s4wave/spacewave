package bifrost_http_listener

import (
	"errors"

	"github.com/aperturerobotics/controllerbus/config"
)

// ControllerID is the controller ID.
const ControllerID = "bifrost/http/listener"

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new listener config.
func NewConfig(addr, clientID string) *Config {
	return &Config{
		Addr:     addr,
		ClientId: clientID,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetCertFile() != "" && c.GetKeyFile() == "" {
		return errors.New("key_file: cannot be empty if cert_file is set")
	}
	if c.GetCertFile() == "" && c.GetKeyFile() != "" {
		return errors.New("cert_file: cannot be empty if key_file is set")
	}
	return nil
}

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

	return ot.EqualVT(c)
}

var _ config.Config = ((*Config)(nil))
