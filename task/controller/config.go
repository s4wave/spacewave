package task_controller

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/world"
	"google.golang.org/protobuf/proto"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new controller config.
// Sets the most important fields only.
func NewConfig(engineID, objectKey string, peerID peer.ID, assignSelf bool) *Config {
	return &Config{
		EngineId:   engineID,
		ObjectKey:  objectKey,
		PeerId:     peerID.Pretty(),
		AssignSelf: assignSelf,
	}
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if len(c.GetPeerId()) == 0 {
		return peer.ErrPeerIDEmpty
	}
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}
	if len(c.GetEngineId()) == 0 {
		return world.ErrEmptyEngineID
	}
	if len(c.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	return nil
}

// ParsePeerID parses the peer ID field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
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
