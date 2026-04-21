package pubsub_relay

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the identifier for the config type.
const ConfigID = ControllerID

// GetConfigID returns the config identifier.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks equality between two configs.
func (c *Config) EqualsConfig(c2 config.Config) bool {
	return config.EqualsConfig[*Config](c, c2)
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if len(c.GetTopicIds()) == 0 {
		return errors.New("at least one topic id required")
	}
	peerID, err := c.ParsePeerID()
	if err != nil {
		return err
	}
	if peerID == "" {
		return errors.New("peer id must be specified")
	}

	return nil
}

// ParsePeerID parses the peer ID if it is not empty.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}
