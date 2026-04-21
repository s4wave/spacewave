package webrtc

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/signaling"
	"github.com/s4wave/spacewave/net/transport"
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
	c.GetWebRtc().ToWebRtcConfiguration()
	if _, err := c.ParseTransportPeerID(); err != nil {
		return err
	}
	if err := c.GetBackoff().Validate(true); err != nil {
		return err
	}
	if _, err := c.ParseBlockPeerIDs(); err != nil {
		return err
	}
	return nil
}

// ParseTransportPeerID parses the node peer ID if it is not empty.
func (c *Config) ParseTransportPeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetTransportPeerId())
}

// ParseBlockPeerIDs parses the remote peer IDs to not allow.
func (c *Config) ParseBlockPeerIDs() ([]peer.ID, error) {
	return confparse.ParsePeerIDsUnique(c.GetBlockPeers(), true)
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

// SetTransportPeerId sets the node peer ID field.
func (c *Config) SetTransportPeerId(peerID string) {
	c.TransportPeerId = peerID
}

// GetDebugVals returns the directive arguments as key/value pairs.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (c *Config) GetDebugVals() config.DebugValues {
	vals := make(config.DebugValues)
	if tp := c.GetTransportPeerId(); tp != "" {
		vals["peer-id"] = []string{tp}
	}
	return vals
}

// _ is a type assertion
var (
	_ transport.Config  = ((*Config)(nil))
	_ config.Debuggable = ((*Config)(nil))
)
