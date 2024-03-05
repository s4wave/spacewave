package plugin_host_controller

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// NewConfig constructs a new controller config.
// Sets the most important fields only.
func NewConfig(
	engineID,
	objectKey,
	volumeID,
	peerID string,
	alwaysFetchManifest bool,
	disableStoreManifest bool,
) *Config {
	return &Config{
		EngineId:  engineID,
		ObjectKey: objectKey,
		PeerId:    peerID,
		VolumeId:  volumeID,

		AlwaysFetchManifest:  alwaysFetchManifest,
		DisableStoreManifest: disableStoreManifest,
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
	if err := c.GetFetchBackoff().Validate(true); err != nil {
		return errors.Wrap(err, "fetch_backoff")
	}
	if err := c.GetExecBackoff().Validate(true); err != nil {
		return errors.Wrap(err, "exec_backoff")
	}
	return nil
}

// ParsePeerID parses the peer ID field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}
