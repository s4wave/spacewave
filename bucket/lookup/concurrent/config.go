package lookup_concurrent

import (
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/bucket"
	lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/pkg/errors"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if err := c.GetBucketConf().Validate(); err != nil {
		return err
	}
	if err := c.GetPutBlockBehavior().Validate(); err != nil {
		return err
	}
	if err := c.GetNotFoundBehavior().Validate(); err != nil {
		return err
	}
	if _, err := c.ParseLookupTimeoutDur(); err != nil {
		return errors.Wrap(err, "lookup_timeout_dur")
	}
	return nil
}

// Validate checks the value.
func (b PutBlockBehavior) Validate() error {
	switch b {
	case PutBlockBehavior_PutBlockBehavior_ALL_VOLUMES:
	case PutBlockBehavior_PutBlockBehavior_NONE:
	default:
		return errors.Errorf("unknown put block behavior: %s", b.String())
	}
	return nil
}

// Validate checks the value.
func (b NotFoundBehavior) Validate() error {
	switch b {
	case NotFoundBehavior_NotFoundBehavior_NONE:
	case NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE:
	default:
		return errors.Errorf("unknown not found behavior: %s", b.String())
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

	return c.EqualVT(ot)
}

// ParseLookupTimeoutDur parses the lookup timeout field.
// Returns 0, nil if empty.
func (c *Config) ParseLookupTimeoutDur() (time.Duration, error) {
	delayStr := c.GetLookupTimeoutDur()
	if delayStr == "" {
		return 0, nil
	}
	return time.ParseDuration(delayStr)
}

// _ is a type assertion
var _ lookup.Config = ((*Config)(nil))
