package dex_solicit

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the identifier for the config type.
const ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBucketId() == "" {
		return errors.New("bucket_id is required")
	}
	if pid := c.GetPeerId(); pid != "" {
		if _, err := confparse.ParsePeerID(pid); err != nil {
			return errors.Wrap(err, "peer_id")
		}
	}
	return nil
}

// ParsePeerID parses the peer ID from config.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
