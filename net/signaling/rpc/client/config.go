package signaling_rpc_client

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/signaling"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if c.GetSignalingId() == "" {
		return signaling.ErrEmptySignalingID
	}
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}
	if err := c.GetClient().Validate(); err != nil {
		return err
	}
	if _, err := c.ParseProtocolID(); err != nil {
		return err
	}
	if err := c.GetBackoff().Validate(true); err != nil {
		return err
	}
	return nil
}

// ParsePeerID parses the session peer ID.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// ParseProtocolID parses the signaling protocol id if it is not empty.
func (c *Config) ParseProtocolID() (protocol.ID, error) {
	return confparse.ParseProtocolID(c.GetProtocolId(), true)
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// GetDebugVals returns the directive arguments as key/value pairs.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (c *Config) GetDebugVals() config.DebugValues {
	vals := make(config.DebugValues)
	if pid, _ := c.ParsePeerID(); pid != "" {
		vals["peer-id"] = []string{pid.String()}
	}
	if tp := c.GetSignalingId(); tp != "" {
		vals["signaling-id"] = []string{tp}
	}
	if tp := c.GetServiceId(); tp != "" {
		vals["service-id"] = []string{tp}
	}
	if tp := c.GetProtocolId(); tp != "" {
		vals["protocol-id"] = []string{tp}
	}
	return vals
}

// _ is a type assertion
var _ config.Debuggable = ((*Config)(nil))
