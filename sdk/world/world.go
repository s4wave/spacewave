// Package s4wave_world is the client contract for a World engine and the
// transactions, states and typed objects reached through it.
//
// EngineResourceService reports the engine, its root snapshots and sequence
// numbers, runs World operations alone or in a batch, opens transactions, and
// watches changes, type counts and object revisions. A transaction or a
// WorldState is a child resource served by WorldStateResourceService, which
// reads, creates and changes objects and graph quads and opens nested Worlds.
// AccessTypedObject resolves an object through its ObjectType into the
// resource that type serves, such as an FSHandle for UnixFS or a Canvas.
//
// A commit that is refused, as when the root is no longer held or another
// writer superseded it, returns a typed WorldErrorCode and committed nothing.
// A commit whose outcome is unknown is never reported as a refusal
// (NewCommitResponse).
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
