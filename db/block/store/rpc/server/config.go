package block_store_rpc_server

import (
	"regexp"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// NewConfig constructs a new config.
func NewConfig(blockStoreID string, write bool, serviceID, serverIdRe string, forceHashType hash.HashType) *Config {
	return &Config{
		BlockStoreId:  blockStoreID,
		Write:         write,
		ServiceId:     serviceID,
		ServerIdRe:    serverIdRe,
		ForceHashType: forceHashType,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBlockStoreId() == "" {
		return block_store.ErrBlockStoreIDEmpty
	}
	if c.GetServiceId() == "" {
		return srpc.ErrEmptyServiceID
	}
	if _, err := c.ParseServerIdRe(); err != nil {
		return errors.Wrap(err, "server_id")
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

// ParseServerIdRe parses the server id regex if set.
// Returns nil, nil if the field was empty.
func (c *Config) ParseServerIdRe() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(c.GetServerIdRe())
}

var _ config.Config = ((*Config)(nil))
