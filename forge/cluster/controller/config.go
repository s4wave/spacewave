package cluster_controller

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new controller config.
// Sets the most important fields only.
func NewConfig(engineID, objectKey string, peerID peer.ID) *Config {
	return &Config{
		EngineId:  engineID,
		ObjectKey: objectKey,
		PeerId:    peerID.String(),
	}
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	// Require a peer identity for the Cluster controller.
	if len(c.GetPeerId()) == 0 {
		return peer.ErrEmptyPeerID
	}

	// Validate the configured peer identity encoding.
	if _, err := c.ParsePeerID(); err != nil {
		return err
	}

	// Require the World engine that stores the Cluster.
	if len(c.GetEngineId()) == 0 {
		return world.ErrEmptyEngineID
	}

	// Require the Cluster object to reconcile.
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
	return config.EqualsConfig(c, other)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
