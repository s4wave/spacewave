package provider_spacewave

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetEndpoint() == "" {
		return errors.New("endpoint is required")
	}
	if _, err := c.ParsePeerID(); err != nil {
		return errors.Wrap(err, "peer_id")
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ControllerID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// ParsePeerID parses the peer id field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
