package space_world_replica

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// ConfigID is the string used to identify this config object.
const ConfigID = "sobject/replica"

// Validate requires the spec, the provider, both peers and the protocol.
func (c *Config) Validate() error {
	// Require the spec and its provider.
	if err := c.GetSpec().Validate(); err != nil {
		return err
	}
	if c.GetProviderId() == "" {
		return errors.New("replica config names no provider")
	}

	// Require two distinct peers and the protocol between them.
	local, err := peer.IDB58Decode(c.GetLocalPeerId())
	if err != nil {
		return errors.Wrap(err, "local peer")
	}
	remote, err := peer.IDB58Decode(c.GetRemotePeerId())
	if err != nil {
		return errors.Wrap(err, "remote peer")
	}
	if local == remote {
		return errors.New("replica nodes are on one Device")
	}
	if c.GetProtocolId() == "" {
		return errors.New("replica config names no protocol")
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)
