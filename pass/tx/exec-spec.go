package pass_tx

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
)

// Validate validates the execution specification.
func (s *ExecSpec) Validate() error {
	// ignores empty peer id
	if _, err := s.ParsePeerID(); err != nil {
		return err
	}
	return nil
}

// ParsePeerID parses the peer ID field.
func (s *ExecSpec) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(s.GetPeerId())
}
