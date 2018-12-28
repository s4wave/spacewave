package reconciler_example

import (
	"errors"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/reconciler"
	"github.com/golang/protobuf/proto"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if c.GetBucketId() == "" {
		return errors.New("bucket id cannot be empty")
	}
	if c.GetVolumeId() == "" {
		return errors.New("volume id cannot be empty")
	}
	if c.GetReconcilerId() == "" {
		return errors.New("reconciler id cannot be empty")
	}
	return nil
}

// SetBucketId sets the bucket ID field.
func (c *Config) SetBucketId(id string) {
	c.BucketId = id
}

// SetVolumeId sets the volume ID field.
func (c *Config) SetVolumeId(id string) {
	c.VolumeId = id
}

// SetReconcilerId sets the reconciler ID field.
func (c *Config) SetReconcilerId(id string) {
	c.ReconcilerId = id
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
var _ reconciler.Config = ((*Config)(nil))
