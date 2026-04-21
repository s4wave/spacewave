package stream_srpc_server_lookup

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate checks the config.
func (c *Config) Validate() error {
	if _, err := confparse.ParsePeerIDs(c.GetPeerIds(), false); err != nil {
		return err
	}
	if _, err := confparse.ParseProtocolIDs(c.GetProtocolIds(), false); err != nil {
		return err
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(c2 config.Config) bool {
	return config.EqualsConfig(c, c2)
}

// GetDebugVals returns the directive arguments as key/value pairs.
func (c *Config) GetDebugVals() config.DebugValues {
	vals := config.DebugValues{
		"protocol-ids": c.GetProtocolIds(),
	}
	if pids := c.GetPeerIds(); len(pids) > 0 {
		vals["peer-ids"] = pids
	}
	if sid := c.GetServerId(); sid != "" {
		vals["server-id"] = []string{sid}
	}
	return vals
}

// _ is a type assertion.
var _ config.Config = ((*Config)(nil))
