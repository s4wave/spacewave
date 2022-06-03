package identity_domain_static

import (
	"github.com/aperturerobotics/controllerbus/config"
	identity "github.com/aperturerobotics/identity"
	"google.golang.org/protobuf/proto"
	"github.com/pkg/errors"
)

// ConfigID is the string used to identify this config object.
const ConfigID = ControllerID

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	for i, d := range c.GetDomains() {
		if err := identity.ValidateDomainID(d); err != nil {
			return errors.Wrapf(err, "domains[%d]", i)
		}
	}
	for ei, ent := range c.GetEntities() {
		if err := ent.Validate(); err != nil {
			return errors.Wrapf(err, "entities[%d]", ei)
		}
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	return proto.Equal(c, other)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
