package identity_domain_server

import (
	"time"

	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/identity"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// ConfigID is the config id used to construct the config.
const ConfigID = ControllerID

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
// Example: controllerbus/example/boilerplate/1
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

// Validate checks the config.
func (c *Config) Validate() error {
	if err := c.GetServer().Validate(); err != nil {
		return err
	}
	for i, did := range c.GetDomainIds() {
		if err := identity.ValidateDomainID(did); err != nil {
			return errors.Wrapf(err, "domain_ids[%d]", i)
		}
	}
	return nil
}

// ParseRequestTimeout parses the request timeout if set.
// Returns 0, nil if not set.
func (c *Config) ParseRequestTimeout() (time.Duration, error) {
	return confparse.ParseDuration(c.GetRequestTimeout())
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
