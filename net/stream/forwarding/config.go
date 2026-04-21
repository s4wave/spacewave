package stream_forwarding

import (
	"errors"

	"github.com/aperturerobotics/controllerbus/config"
	ma "github.com/aperturerobotics/go-multiaddr"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}

	pid := protocol.ID(c.GetProtocolId())
	if err := pid.Validate(); err != nil {
		return err
	}

	if c.GetTargetMultiaddr() == "" {
		return errors.New("target multiaddress cannot be nil")
	}

	if _, err := c.ParseTargetMultiaddr(); err != nil {
		return err
	}

	return nil
}

// ParsePeerID parses the peer ID.
// may return nil.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// ParseTargetMultiaddr parses the multiaddress.
func (c *Config) ParseTargetMultiaddr() (ma.Multiaddr, error) {
	return ma.NewMultiaddr(c.GetTargetMultiaddr())
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(c2 config.Config) bool {
	return config.EqualsConfig[*Config](c, c2)
}

var _ config.Config = ((*Config)(nil))
