package forge_execution

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/pkg/errors"
)

// CheckPeerID checks if the peer ID matches the Execution.
func (e *Execution) CheckPeerID(id peer.ID) error {
	// accept any peer id if field is unset
	if len(e.GetPeerId()) == 0 {
		return nil
	}

	currPeerID, err := e.ParsePeerID()
	if err != nil {
		return err
	}

	// basic string comparison
	currPeerIDStr := currPeerID.Pretty()
	idStr := id.Pretty()
	if currPeerIDStr != idStr {
		return errors.Wrapf(ErrUnexpectedPeerID, "expected %s got %s", currPeerIDStr, idStr)
	}

	// match
	return nil
}

// ParsePeerID parses the peer ID field.
// Returns empty if not set.
func (e *Execution) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(e.GetPeerId())
}
