package identity_domain_client

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// ConfigID identifies the config.
const ConfigID = ControllerID

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// ParsePeerID parses the peer id field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if err := c.GetDomainInfo().Validate(); err != nil {
		return err
	}
	if err := c.GetClientOpts().Validate(); err != nil {
		return errors.Wrap(err, "client_opts")
	}
	return nil
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ov, ok := other.(*Config)
	if !ok {
		return false
	}
	return proto.Equal(c, ov)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
