package identity_domain_client

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// ConfigID is the config id used to construct the config.
const ConfigID = ControllerID

// GetConfigID returns the unique string for this configuration type.
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
	if err := c.GetClientOpts().Validate(); err != nil {
		return errors.Wrap(err, "client_opts")
	}
	return nil
}

// ParsePeerID parses the peer id field.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
