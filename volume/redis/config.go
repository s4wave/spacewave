package volume_redis

import (
	"net/url"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// BuildRedisOptions builds redis options from the config.
func (c *Config) BuildRedisOptions() ([]redis.DialOption, error) {
	return []redis.DialOption{redis.DialClientName("bifrost")}, nil
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if _, err := c.BuildRedisOptions(); err != nil {
		return err
	}

	rawurl := c.GetUrl()
	if rawurl == "" {
		return errors.New("redis url required")
	}

	u, err := url.Parse(rawurl)
	if err != nil {
		return err
	}

	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return errors.Errorf("invalid redis URL scheme: %s", u.Scheme)
	}

	if u.Opaque != "" {
		return errors.Errorf("invalid redis URL, url is opaque: %s", rawurl)
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
