package lookup_concurrent

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/bucket"
	lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/golang/protobuf/proto"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if err := c.GetBucketConf().Validate(); err != nil {
		return err
	}
	return nil
}

// SetBucketConf sets the bucket config.
func (c *Config) SetBucketConf(f *bucket.Config) {
	c.BucketConf = f
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

	return proto.Equal(c, ot)
}

// _ is a type assertion
var _ lookup.Config = ((*Config)(nil))
