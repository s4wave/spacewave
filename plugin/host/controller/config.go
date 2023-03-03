package plugin_host_controller

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/hydra/world"
)

// NewConfig constructs a new controller config.
// Sets the most important fields only.
func NewConfig(distPlatformID, engineID, objectKey, volumeID, peerID string) *Config {
	return &Config{
		DistPlatformId: distPlatformID,
		EngineId:       engineID,
		ObjectKey:      objectKey,
		PeerId:         peerID,
		VolumeId:       volumeID,
	}
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
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
	if len(c.GetObjectKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if len(c.GetVolumeId()) == 0 {
		return volume.ErrVolumeIDEmpty
	}
	return nil
}

// ParsePeerID parses the peer ID field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}
