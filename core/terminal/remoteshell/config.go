package remoteshell

import "github.com/aperturerobotics/controllerbus/config"

// ControllerID identifies the remote shell controller.
const ControllerID = "spacewave/device/remote-shell"

// ConfigID identifies the remote shell controller config.
const ConfigID = ControllerID

// Validate accepts any config, since the controller has no settings.
func (c *Config) Validate() error {
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(c2 config.Config) bool {
	return config.EqualsConfig[*Config](c, c2)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
