package plugin_host_scheduler

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/backoff"
	"github.com/pkg/errors"
)

// ConfigID is the config identifier.
const ConfigID = ControllerID

// NewConfig constructs a new controller config.
// Sets the most important fields only.
func NewConfig(
	engineID,
	objectKey,
	volumeID,
	peerID string,
	watchFetchManifest,
	disableStoreManifest,
	disableCopyManifest bool,
) *Config {
	return &Config{
		EngineId:  engineID,
		ObjectKey: objectKey,
		PeerId:    peerID,
		VolumeId:  volumeID,

		WatchFetchManifest:   watchFetchManifest,
		DisableStoreManifest: disableStoreManifest,
		DisableCopyManifest:  disableCopyManifest,
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

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// ParsePeerID parses the peer ID field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// BuildExecBackoff gets the ExecBackoff and fills defaults if applicable.
func (c *Config) BuildExecBackoff() *backoff.Backoff {
	backoffConf := c.GetExecBackoff().CloneVT()
	if backoffConf == nil {
		backoffConf = &backoff.Backoff{}
	}
	if backoffConf.BackoffKind == 0 {
		if backoffConf.Exponential == nil {
			backoffConf.Exponential = &backoff.Exponential{}
		}
		backoffConf.BackoffKind = backoff.BackoffKind_BackoffKind_EXPONENTIAL
		backoffConf.Exponential.MaxInterval = 2100
	}
	return backoffConf
}

// BuildFetchBackoff gets the FetchBackoff and fills defaults if applicable.
func (c *Config) BuildFetchBackoff() *backoff.Backoff {
	backoffConf := c.GetFetchBackoff().CloneVT()
	if backoffConf == nil {
		backoffConf = &backoff.Backoff{}
	}
	if backoffConf.BackoffKind == 0 {
		if backoffConf.Exponential == nil {
			backoffConf.Exponential = &backoff.Exponential{}
		}
		backoffConf.BackoffKind = backoff.BackoffKind_BackoffKind_EXPONENTIAL
		backoffConf.Exponential.MaxInterval = 1200
	}
	return backoffConf
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
