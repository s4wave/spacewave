package stream_api_dial

import (
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	// Require and parse the target peer ID.
	if c.GetPeerId() == "" {
		return peer.ErrEmptyPeerID
	}

	// Validate the optional local peer ID constraint.
	if _, err := c.ParseLocalPeerID(); err != nil {
		return err
	}

	// Validate the stream protocol identifier.
	pid := protocol.ID(c.GetProtocolId())
	if err := pid.Validate(); err != nil {
		return err
	}

	return nil
}

// ParseLocalPeerID parses the local peer ID constraint.
// may be empty.
func (c *Config) ParseLocalPeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetLocalPeerId())
}

// ParsePeerID parses the target peer ID constraint.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}
