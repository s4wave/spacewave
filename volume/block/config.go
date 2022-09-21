package volume_block

import (
	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/pkg/errors"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// NewConfig constructs a new block volume config.
func NewConfig(
	volumeID, bucketID, objectStoreID string,
	initHeadRef *bucket.ObjectRef,
	stateTransformConf *block_transform.Config,
) *Config {
	return &Config{
		BucketId:           bucketID,
		VolumeId:           volumeID,
		ObjectStoreId:      objectStoreID,
		InitHeadRef:        initHeadRef,
		StateTransformConf: stateTransformConf,
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
	if err := c.GetStateTransformConf().Validate(); err != nil {
		return errors.Wrap(err, "state_transform_conf")
	}

	if c.GetVolumeId() == "" {
		return errors.New(
			"block volume requires volume_id to be set for writes",
		)
	}

	if err := c.GetKvKeyOpts().Validate(); err != nil {
		return err
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
	ot, ok := other.(*Config)
	if !ok {
		return false
	}

	return c.EqualVT(ot)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
