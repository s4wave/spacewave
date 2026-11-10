package world_block

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// InsertGraphQuads inserts new, unique relationships in one graph index update.
// It validates all endpoints before writing and records the same World changes
// as individual SetGraphQuad calls. It writes only the graph: the records of the
// objects a relationship names keep their revisions. An existing or
// repeated relationship is an error; discard the enclosing transaction on error.
// Like other mutable WorldState operations, calls must be serialized by its owner.
func (t *WorldState) InsertGraphQuads(ctx context.Context, quads []world.GraphQuad) error {
	// Reject a non-write, discarded state, or canceled context.
	if !t.write {
		return tx.ErrNotWrite
	}
	if err := t.discardErr(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil || len(quads) == 0 {
		return err
	}

	// Cayley drops some repeats within one call, so reject them first.
	if err := requireUniqueGraphQuads(quads); err != nil {
		return err
	}

	// Build the Cayley deltas and require each endpoint once.
	deltas := make([]graph.Delta, len(quads))
	checked := make(map[string]struct{})
	for i, q := range quads {
		if err := ctx.Err(); err != nil {
			return err
		}
		cq, err := world.GraphQuadToCayleyQuad(q, true)
		if err != nil {
			return err
		}
		deltas[i] = graph.Delta{Quad: cq, Action: graph.Add}
		for _, value := range [2]string{q.GetSubject(), q.GetObj()} {
			key, err := world.GraphValueToKey(value)
			if err != nil {
				return err
			}
			if _, ok := checked[key]; ok {
				continue
			}
			found, err := t.HasObject(ctx, key)
			if err != nil {
				return err
			}
			if !found {
				return world.ErrObjectNotFound
			}
			checked[key] = struct{}{}
		}
	}

	// Apply the deltas to the graph index.
	graphHd, err := t.graph(ctx)
	if err != nil {
		return err
	}
	if err := graphHd.ApplyDeltas(ctx, deltas, graph.IgnoreOpts{}); err != nil {
		return err
	}

	// Record one graph-set World change per relationship.
	for _, q := range quads {
		if _, err := t.queueWorldChange(ctx, &WorldChange{
			ChangeType: WorldChangeType_WorldChange_GRAPH_SET,
			Quad:       world.GraphQuadToQuad(q),
		}); err != nil {
			return err
		}
	}
	return nil
}

// requireUniqueGraphQuads rejects a relationship repeated within quads. Graph
// values are raw Cayley terms, so equal terms are the same relationship.
func requireUniqueGraphQuads(quads []world.GraphQuad) error {
	sorted := slices.SortedFunc(slices.Values(quads), compareGraphQuads)
	for i := 1; i < len(sorted); i++ {
		if compareGraphQuads(sorted[i-1], sorted[i]) != 0 {
			continue
		}
		cq, err := world.GraphQuadToCayleyQuad(sorted[i], false)
		if err != nil {
			return err
		}
		return &graph.DeltaError{Delta: graph.Delta{Quad: cq, Action: graph.Add}, Err: graph.ErrQuadExists}
	}
	return nil
}

// compareGraphQuads orders relationships by subject, predicate, object and
// label.
func compareGraphQuads(a, b world.GraphQuad) int {
	return cmp.Or(
		strings.Compare(a.GetSubject(), b.GetSubject()),
		strings.Compare(a.GetPredicate(), b.GetPredicate()),
		strings.Compare(a.GetObj(), b.GetObj()),
		strings.Compare(a.GetLabel(), b.GetLabel()),
	)
}
