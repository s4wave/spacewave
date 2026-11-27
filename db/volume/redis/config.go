package volume_redis

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/gomodule/redigo/redis"
	store_kvtx_redis "github.com/s4wave/spacewave/db/store/kvtx/redis"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

const redisClientName = "bifrost"

// BuildRedisOptions builds redis options from the config.
func (c *Config) BuildRedisOptions() ([]redis.DialOption, error) {
	return []redis.DialOption{
		redis.DialConnectTimeout(store_kvtx_redis.LeaseRenewInterval),
		redis.DialReadTimeout(store_kvtx_redis.LeaseRenewInterval),
		redis.DialWriteTimeout(store_kvtx_redis.LeaseRenewInterval),
	}, nil
}

// Validate checks the volume, store and Redis connection configuration.
func (c *Config) Validate() error {
	if _, err := c.BuildRedisOptions(); err != nil {
		return err
	}

	if err := c.GetClient().Validate(); err != nil {
		return err
	}

	if err := c.GetVolumeConfig().Validate(); err != nil {
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
	return config.EqualsConfig[*Config](c, other)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
