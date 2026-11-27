package world_block

import (
	"maps"
	"slices"
	"sync"

	"github.com/s4wave/spacewave/db/world"
)

// changeSummary owns a sealed transaction's immutable touched-name maps.
// Only feed consumers materialize it, after the writer has handed it off.
type changeSummary struct {
	// keys holds the touched object names.
	keys map[string]struct{}
	// quads holds the distinct touched relationships.
	quads map[quadKey]struct{}
	// unknown reports an overflow or unrepresentable relationship.
	unknown bool
	// once serializes materialization among feed and catch-up consumers.
	once sync.Once
	// changes is the immutable materialized summary.
	changes *world.ChangeSet
}

// materialize sorts names and constructs relationship records once, off the writer.
func (s *changeSummary) materialize() *world.ChangeSet {
	// Missing evidence is Unknown, never empty: subscribers must refresh when
	// the feed cannot prove the summary of an actual transition.
	if s == nil {
		return world.NewUnknownChangeSet()
	}

	// Share one materialization among consumers of the exact transition.
	s.once.Do(func() {
		// Preserve overflow as Unknown rather than constructing a partial summary.
		if s.unknown {
			s.changes = world.NewUnknownChangeSet()
			return
		}

		// Materialize the immutable names once, then release the staging maps.
		changes := &world.ChangeSet{Keys: slices.Sorted(maps.Keys(s.keys))}
		for q := range s.quads {
			changes.Quads = append(changes.Quads, world.NewGraphQuad(q.subj, q.pred, q.obj, q.label))
		}
		s.changes = changes
		s.keys, s.quads = nil, nil
	})
	return s.changes
}
