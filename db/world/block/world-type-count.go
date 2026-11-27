package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// noteTypeChange adds the effect of a queued change on the type counts. Every
// graph write queues one change per quad it adds or removes, so the counts
// follow the type edges exactly.
func (t *WorldState) noteTypeChange(w *WorldChange) {
	// A set adds one and a delete removes one. Other changes leave no type.
	var delta int64
	switch w.GetChangeType() {
	case WorldChangeType_WorldChange_GRAPH_SET:
		delta = 1
	case WorldChangeType_WorldChange_GRAPH_DELETE:
		delta = -1
	default:
		return
	}
	q := w.GetQuad()
	typeID, ok := world.TypeEdgeTypeID(q.GetPredicate(), q.GetObj())
	if !ok {
		return
	}
	if t.typeDeltas == nil {
		t.typeDeltas = make(map[string]int64)
	}
	t.typeDeltas[typeID] += delta
}

// flushTypeCounts adds the pending type deltas to the root's counts.
func (t *WorldState) flushTypeCounts(r *World) {
	if len(t.typeDeltas) == 0 {
		return
	}
	r.TypeCounts = world.AddTypeCounts(r.GetTypeCounts(), t.typeDeltas)
	t.typeDeltas = nil
}

// ListTypeCounts returns the root's type counts plus the pending changes'.
func (t *WorldState) ListTypeCounts(ctx context.Context) ([]*world.TypeCount, error) {
	if err := t.discardErr(); err != nil {
		return nil, err
	}
	r, err := t.GetRoot(ctx)
	if err != nil {
		return nil, err
	}
	return world.AddTypeCounts(r.GetTypeCounts(), t.typeDeltas), nil
}
