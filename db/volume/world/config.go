package volume_world

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// NewConfig constructs a new block volume config.
func NewConfig(
	volumeID, bucketID, engineID, objectKey string,
	initHeadRef *bucket.ObjectRef,
) *Config {
	return &Config{
		VolumeId:    volumeID,
		BucketId:    bucketID,
		EngineId:    engineID,
		ObjectKey:   objectKey,
		InitHeadRef: initHeadRef,
	}
}

// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	initRef := c.GetInitHeadRef()
	hasInitRef := !initRef.GetEmpty()
	if hasInitRef {
		if err := c.GetInitHeadRef().Validate(); err != nil {
			return errors.Wrap(err, "init_head_ref")
		}
	}
	if c.GetVolumeId() == "" {
		return errors.New(
			"block volume requires volume_id to be set for writes",
		)
	}
	if err := c.GetKvKeyOpts().Validate(); err != nil {
		return err
	}
	if c.GetEngineId() == "" {
		return world.ErrEmptyEngineID
	}

	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ControllerID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
