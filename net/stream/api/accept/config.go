package stream_api_accept

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if c.GetLocalPeerId() != "" {
		if _, err := c.ParseLocalPeerID(); err != nil {
			return err
		}
	}

	if pids := c.GetRemotePeerIds(); len(pids) != 0 {
		for _, pid := range pids {
			if _, err := confparse.ParsePeerID(pid); err != nil {
				return err
			}
		}
	}

	pid := protocol.ID(c.GetProtocolId())
	if err := pid.Validate(); err != nil {
		return err
	}

	return nil
}

// ParseLocalPeerID parses the local peer ID.
// may return nil.
func (c *Config) ParseLocalPeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetLocalPeerId())
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
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
