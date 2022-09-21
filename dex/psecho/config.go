package psecho

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
)

// ConfigID is the identifier for the config type.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if chn := c.GetPubsubChannel(); chn == "" {
		return errors.New("pubsub channel must be specified")
	}
	if _, err := c.ParsePeerID(); err != nil {
		return errors.Wrap(err, "parse peer id")
	}
	return nil
}

// ParsePeerID parses the target peer ID constraint.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
// Example: bifrost/transport/udp/1
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	oc, ok := other.(*Config)
	if !ok {
		return false
	}
	return c.EqualVT(oc)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
