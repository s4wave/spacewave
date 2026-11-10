package world_block_engine

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new block world engine config.
func NewConfig(
	engineID, volumeID, bucketID, objectStoreID string,
	initHeadRef *bucket.ObjectRef,
	stateTransformConf *block_transform.Config,
	enableChangelog bool,
) *Config {
	return &Config{
		BucketId:           bucketID,
		EngineId:           engineID,
		VolumeId:           volumeID,
		ObjectStoreId:      objectStoreID,
		InitHeadRef:        initHeadRef,
		StateTransformConf: stateTransformConf,
		DisableChangelog:   !enableChangelog,
	}
}

// Validate checks that the configuration's values look correct.
func (c *Config) Validate() error {
	// Determine whether the World configuration supplies an initial head.
	initRef := c.GetInitHeadRef()
	hasInitRef := !initRef.GetEmpty()

	// Validate the initial World head and its persisted-state transform.
	if hasInitRef {
		if err := c.GetInitHeadRef().Validate(); err != nil {
			return errors.Wrap(err, "init_head_ref")
		}
	}
	if err := c.GetStateTransformConf().Validate(); err != nil {
		return errors.Wrap(err, "state_transform_conf")
	}
	if err := c.GetFormat().ValidateInitial(c.GetGraphSettings()); err != nil {
		return errors.Wrap(err, "format")
	}

	// A regeneration writes the head's whole graph, so it neither runs on a
	// World with no persisted head nor beside a graph settings change.
	if err := c.GetRegen().Validate(); err != nil {
		return errors.Wrap(err, "regen")
	}
	if c.GetRegen().GetTarget() != RegenTarget_REGEN_TARGET_NONE {
		if c.GetEphemeral() {
			return errors.New("regen: an ephemeral World keeps no head to regenerate")
		}
		if len(c.GetGraphSettings().GetIndexes()) != 0 {
			return errors.New("regen: graph_settings would change the graph being regenerated")
		}
	}

	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
