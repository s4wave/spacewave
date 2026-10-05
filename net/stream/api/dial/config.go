package stream_api_dial

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// Validate checks that the values are well formed and that the target is not
// the local peer.
func (c *Config) Validate() error {
	// Require the target peer ID.
	if c.GetPeerId() == "" {
		return peer.ErrEmptyPeerID
	}

	// Validate the optional local peer ID constraint. A stream to the local
	// peer would wait forever: no transport links a peer to itself.
	localPeerID, err := c.ParseLocalPeerID()
	if err != nil {
		return err
	}
	if localPeerID != "" && localPeerID.String() == c.GetPeerId() {
		return errors.Errorf("cannot dial the local peer %s", localPeerID)
	}

	// Validate the stream protocol identifier.
	pid := protocol.ID(c.GetProtocolId())
	if err := pid.Validate(); err != nil {
		return err
	}

	return nil
}

// ParseLocalPeerID parses the local peer ID constraint, which may be empty.
func (c *Config) ParseLocalPeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetLocalPeerId())
}

// ParsePeerID parses the target peer ID constraint.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}
