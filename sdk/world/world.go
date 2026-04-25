package s4wave_world

import (
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ParsePeerID parses the op sender peer ID.
func (r *ApplyWorldOpRequest) ParsePeerID() (peer.ID, error) {
	// GetOpSender can be empty
	return confparse.ParsePeerID(r.GetOpSender())
}

// ParsePeerID parses the op sender peer ID.
func (r *ApplyObjectOpRequest) ParsePeerID() (peer.ID, error) {
	// GetOpSender can be empty
	return confparse.ParsePeerID(r.GetOpSender())
}
