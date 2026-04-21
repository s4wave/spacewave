package link_establish_controller

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

// ParsePeerIDs parses the peer ids field.
func (c *Config) ParsePeerIDs() ([]peer.ID, error) {
	return confparse.ParsePeerIDs(c.GetPeerIds(), false)
}

// SetPeerIDs sets the peer ids field.
func (c *Config) SetPeerIDs(ids []peer.ID) {
	pids := make([]string, len(ids))
	for i, pid := range ids {
		pids[i] = peer.IDB58Encode(pid)
	}
	c.PeerIds = pids
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if len(c.GetPeerIds()) == 0 {
		return errors.New("at least one peer id required")
	}
	if _, err := c.ParsePeerIDs(); err != nil {
		return errors.Wrap(err, "peer_ids")
	}
	if _, err := c.ParseSrcPeerId(); err != nil {
		return errors.Wrap(err, "src_peer_id")
	}
	return nil
}

// ParseSrcPeerId parses the source peer id.
// May return empty.
func (c *Config) ParseSrcPeerId() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetSrcPeerId())
}
