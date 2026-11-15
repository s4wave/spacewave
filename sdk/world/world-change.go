package s4wave_world

import (
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/world"
)

// NewWorldChange encodes the changes that reach seqno for WatchChanges.
func NewWorldChange(seqno uint64, changes *world.ChangeSet) *WorldChange {
	// Send an unknown change without its partial lists.
	if changes.Unknown {
		return &WorldChange{Seqno: seqno, Unknown: true}
	}

	// Encode each changed quad with its stored spelling.
	quads := make([]*quad.Quad, len(changes.Quads))
	for i, q := range changes.Quads {
		quads[i] = &quad.Quad{
			Subject:   q.GetSubject(),
			Predicate: q.GetPredicate(),
			Obj:       q.GetObj(),
			Label:     q.GetLabel(),
		}
	}
	return &WorldChange{Seqno: seqno, Keys: changes.Keys, Quads: quads}
}

// NewChangeResponse wraps the changes that reach seqno as a WatchChanges
// response.
func NewChangeResponse(seqno uint64, changes *world.ChangeSet) *WatchChangesResponse {
	return &WatchChangesResponse{Body: &WatchChangesResponse_Change{Change: NewWorldChange(seqno, changes)}}
}

// NewInterestAckResponse acknowledges every interest delta up to version: the
// changes after afterSeqno are filtered under the interest they leave.
func NewInterestAckResponse(version, afterSeqno uint64) *WatchChangesResponse {
	return &WatchChangesResponse{Body: &WatchChangesResponse_InterestAck{
		InterestAck: &InterestAck{Version: version, AfterSeqno: afterSeqno},
	}}
}

// NewChangeInterestDelta returns the delta that adds keys and patterns, at
// version.
func NewChangeInterestDelta(version uint64, keys []string, patterns []world.GraphQuad) *ChangeInterestDelta {
	quads := make([]*quad.Quad, len(patterns))
	for i, q := range patterns {
		quads[i] = &quad.Quad{
			Subject:   q.GetSubject(),
			Predicate: q.GetPredicate(),
			Obj:       q.GetObj(),
			Label:     q.GetLabel(),
		}
	}
	return &ChangeInterestDelta{Version: version, AddKeys: keys, AddPatterns: quads}
}

// ToChangeSet decodes the changes the message lists.
func (c *WorldChange) ToChangeSet() *world.ChangeSet {
	// Decode an unknown change without its lists.
	if c.GetUnknown() {
		return world.NewUnknownChangeSet()
	}

	// Expose the quads through the World graph interface.
	quads := make([]world.GraphQuad, len(c.GetQuads()))
	for i, q := range c.GetQuads() {
		quads[i] = q
	}
	return &world.ChangeSet{Keys: c.GetKeys(), Quads: quads}
}
