package bucket_http_server

import (
	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/pkg/errors"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(bucketID, volumeID string, write bool, pathPrefix string, forceHashType hash.HashType) *Config {
	return &Config{
		BucketId:      bucketID,
		VolumeId:      volumeID,
		Write:         write,
		PathPrefix:    pathPrefix,
		ForceHashType: forceHashType,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBucketId() == "" {
		return bucket.ErrBucketIDEmpty
	}
	if err := c.GetForceHashType().Validate(); err != nil {
		return errors.Wrap(err, "force_hash_type")
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

var _ config.Config = ((*Config)(nil))
