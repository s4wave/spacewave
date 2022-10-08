package plugin_static

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/hydra/world"
)

// NewConfig constructs a new controller config.
func NewConfig(engineID, pluginHostKey string, peerID string) *Config {
	return &Config{
		EngineId:      engineID,
		PluginHostKey: pluginHostKey,
		PeerId:        peerID,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if len(c.GetPeerId()) == 0 {
		return peer.ErrEmptyPeerID
	}
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}
	if len(c.GetEngineId()) == 0 {
		return world.ErrEmptyEngineID
	}
	if len(c.GetPluginHostKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	return nil
}

// ParsePeerID parses the peer ID field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}
