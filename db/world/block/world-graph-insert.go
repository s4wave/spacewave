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
// as individual SetGraphQuad calls. Each subject must name an existing object,
// the holder of the relationship; an object may name a key not written yet, so
// a record can reference a target created later in the transaction or after
// it. DeleteObject removes the quads of a key in both directions, including
// those written before the key's object existed. It writes only the graph: the
// records of the objects a relationship names keep their revisions. An
// existing or repeated relationship is an error; discard the enclosing
// transaction on error. Like other mutable WorldState operations, calls must
// be serialized by its owner.
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

	// Build the Cayley deltas, require both endpoints to be object keys, and
	// require each subject to exist once.
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
		if _, err := world.GraphValueToKey(q.GetObj()); err != nil {
			return err
		}
		subject, err := world.GraphValueToKey(q.GetSubject())
		if err != nil {
			return err
		}
		if _, ok := checked[subject]; ok {
			continue
		}
		found, err := t.HasObject(ctx, subject)
		if err != nil {
			return err
		}
		if !found {
			return world.ErrObjectNotFound
		}
		checked[subject] = struct{}{}
	}

	// Apply the deltas to the graph index.
	graphHd, err := t.graph(ctx)
	if err != nil {
		return err
	}
	if err := graphHd.ApplyDeltas(ctx, deltas, graph.IgnoreOpts{}); err != nil {
		return t.failGraphCall(err)
	}

	// Record one graph-set World change per relationship.
	for _, q := range quads {
		if _, err := t.queueWorldChange(ctx, &WorldChange{
			ChangeType: WorldChangeType_WorldChange_GRAPH_SET,
			Quad:       world.GraphQuadToQuad(q),
		}); err != nil {
			return t.failWrite(err)
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
