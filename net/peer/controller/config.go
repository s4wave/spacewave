package peer_controller

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ConfigID is the identifier for the config type.
const ConfigID = ControllerID

// NewConfigWithPrivKey builds a new configuration with a private key
func NewConfigWithPrivKey(pk crypto.PrivKey) (*Config, error) {
	privKeyStr, err := confparse.MarshalPrivateKey(pk)
	if err != nil {
		return nil, err
	}

	return &Config{
		PrivKey: privKeyStr,
	}, nil
}

// GetConfigID returns the config identifier.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks equality between two configs.
func (c *Config) EqualsConfig(c2 config.Config) bool {
	oc, ok := c2.(*Config)
	if !ok {
		return false
	}

	if c.GetPrivKey() == oc.GetPrivKey() {
		return true
	}

	pk1, err := c.ParsePrivateKey()
	if err != nil {
		return false
	}

	pk2, err := oc.ParsePrivateKey()
	if err != nil {
		return false
	}

	return pk2.Equals(pk1)
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.SizeVT() == 0 {
		return nil
	}

	if _, err := c.ParseToPeer(); err != nil {
		return err
	}

	return nil
}

// ParsePrivateKey parses the private key from the configuration.
// Returns nil, nil if unset.
func (c *Config) ParsePrivateKey() (crypto.PrivKey, error) {
	return confparse.ParsePrivateKey(c.GetPrivKey())
}

// ParsePublicKey parses the public key from the configuration.
// Returns nil, nil if unset.
func (c *Config) ParsePublicKey() (crypto.PubKey, error) {
	return confparse.ParsePublicKey(c.GetPubKey())
}

// ParsePeerID parses the peer ID.
// may return nil.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// ParseToPeer parses the fields and builds the corresponding Peer.
func (c *Config) ParseToPeer() (peer.Peer, error) {
	if c.SizeVT() == 0 {
		return peer.NewPeer(nil)
	}
	return confparse.ParsePeer(c.GetPrivKey(), c.GetPubKey(), c.GetPeerId())
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
