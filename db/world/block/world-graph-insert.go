package world_block

import (
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// InsertGraphQuads inserts new, unique relationships in one graph index update.
// It validates all endpoints before writing and records the same endpoint
// revisions and World changes as individual SetGraphQuad calls. An existing or
// repeated relationship is an error; discard the enclosing transaction on error.
// Like other mutable WorldState operations, calls must be serialized by its owner.
func (t *WorldState) InsertGraphQuads(ctx context.Context, quads []world.GraphQuad) error {
	if !t.write {
		return tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return tx.ErrDiscarded
	}
	if err := ctx.Err(); err != nil || len(quads) == 0 {
		return err
	}

	seen := make(map[string]struct{}, len(quads))
	deltas := make([]graph.Delta, len(quads))
	endpoints := make([][2]*ObjectState, len(quads))
	for i, q := range quads {
		cq, err := world.GraphQuadToCayleyQuad(q, true)
		if err != nil {
			return err
		}
		deltas[i] = graph.Delta{Quad: cq, Action: graph.Add}
		key := cq.NQuad()
		if _, exists := seen[key]; exists {
			return &graph.DeltaError{Delta: deltas[i], Err: graph.ErrQuadExists}
		}
		seen[key] = struct{}{}
		for j, value := range [2]string{q.GetSubject(), q.GetObj()} {
			key, err := world.GraphValueToKey(value)
			if err != nil {
				return err
			}
			endpoints[i][j], err = t.mustGetObject(ctx, key)
			if err != nil {
				return err
			}
		}
	}
	if err := t.graphHd.ApplyDeltas(ctx, deltas, graph.IgnoreOpts{}); err != nil {
		return err
	}

	for i, q := range quads {
		for _, endpoint := range endpoints[i] {
			if _, err := endpoint.incrementRev(ctx, false); err != nil {
				return err
			}
		}
		if _, err := t.queueWorldChange(ctx, &WorldChange{
			ChangeType: WorldChangeType_WorldChange_GRAPH_SET,
			Quad:       world.GraphQuadToQuad(q),
		}); err != nil {
			return err
		}
	}
	return nil
}
