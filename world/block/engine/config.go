package world_block_engine

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new block world engine config.
func NewConfig(
	engineID, volumeID, bucketID, objectStoreID string,
	initHeadRef *bucket.ObjectRef,
) *Config {
	return &Config{
		BucketId:      bucketID,
		EngineId:      engineID,
		VolumeId:      volumeID,
		ObjectStoreId: objectStoreID,
		InitHeadRef:   initHeadRef,
	}
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	initRef := c.GetInitHeadRef()
	hasInitRef := !initRef.GetEmpty()

	if hasInitRef {
		if err := c.GetInitHeadRef().Validate(); err != nil {
			return errors.Wrap(err, "init_head_ref")
		}
	}

	/*
		if c.GetVolumeId() == "" || c.GetObjectStoreId() == "" {
			if !hasInitRef || initRef.GetBucketId() == "" {
				return errors.New(
					"world engine requires a init ref with a bucket id if the volume id or object store id are not set",
				)
			}
		}
	*/

	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}

	return proto.Equal(ot, c)
}

var _ config.Config = ((*Config)(nil))
