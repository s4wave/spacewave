package auth_challenge_client

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/golang/protobuf/proto"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}
	if _, err := c.ParseServerPeerIDs(); err != nil {
		return err
	}
	/*
		if len(l) == 0 {
			errors.New("server peer ids must be set")
		}
	*/
	return nil
}

// ParsePeerID parses the peer ID.
// may return nil.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// ParseServerPeerIDs parses the server list.
func (c *Config) ParseServerPeerIDs() (map[peer.ID]struct{}, error) {
	ids := make(map[peer.ID]struct{})
	for _, id := range c.GetServerPeerIds() {
		pid, err := peer.IDB58Decode(id)
		if err != nil {
			return nil, err
		}
		ids[pid] = struct{}{}
	}
	return ids, nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	return proto.Equal(c, other)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
