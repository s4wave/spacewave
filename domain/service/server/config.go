package identity_domain_server

import (
	"time"

	"github.com/aperturerobotics/bifrost/peer"
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
	if _, err := c.ParsePeerIds(); err != nil {
		return err
	}
	for i, did := range c.GetDomainIds() {
		if err := identity.ValidateDomainID(did); err != nil {
			return errors.Wrapf(err, "domain_ids[%d]", i)
		}
	}
	return nil
}

// ParsePeerIds parses the peer ids to listen on.
func (c *Config) ParsePeerIds() ([]peer.ID, error) {
	out := make([]peer.ID, len(c.GetPeerIds()))
	var err error
	for i, peerIDStr := range c.GetPeerIds() {
		out[i], err = confparse.ParsePeerID(peerIDStr)
		if err != nil {
			return nil, err
		}
		if out[i] == "" {
			return nil, errors.Wrapf(peer.ErrEmptyPeerID, "peer_ids[%d]", i)
		}
	}
	return out, nil
}

// ParseRequestTimeout parses the request timeout if set.
// Returns 0, nil if not set.
func (c *Config) ParseRequestTimeout() (time.Duration, error) {
	return confparse.ParseDuration(c.GetRequestTimeout())
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
