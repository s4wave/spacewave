package world_block

import (
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// graphQuadBatchCollector resolves filters within one underlying transaction.
type graphQuadBatchCollector interface {
	// CollectFilteredQuadsBatch resolves each filter with an independent limit.
	CollectFilteredQuadsBatch(ctx context.Context, filters []quad.Quad, limitPerFilter uint32) ([][]quad.Quad, error)
}

// graphPathReadOperation shares graph lookups within one World read operation.
type graphPathReadOperation struct {
	*WorldState

	graphHd world.CayleyHandle
}

// accessCayleyGraph lends the current handle for the callback's lifetime.
func (t *WorldState) accessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	if t.discarded.Load() {
		return tx.ErrDiscarded
	}

	hd := t.graphHd
	// Direct graph mutations currently bypass the World changelog.
	return cb(ctx, hd)
}

// lookupGraphQuadsOnWorld resolves one filter through the batch lookup path.
func (t *WorldState) lookupGraphQuadsOnWorld(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	filters := [1]world.GraphQuad{filter}
	results, err := t.lookupGraphQuadsBatchOnWorld(ctx, filters[:], limit)
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// lookupGraphQuadsBatchOnWorld shares a storage read scope for read-only Worlds.
func (t *WorldState) lookupGraphQuadsBatchOnWorld(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	if t.discarded.Load() {
		return nil, tx.ErrDiscarded
	}
	if !t.write && t.store != nil {
		store, release, err := t.store.BeginReadOperation(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
		ctx = block.WithReadOperationStore(ctx, store)
	}

	var graphHd world.CayleyHandle = t.graphHd
	collector, _ := t.graphHd.QuadStore.(graphQuadBatchCollector)
	if collector == nil || len(filters) != 1 {
		graphHd = world.NewReadOperationCayleyHandle(graphHd)
	}
	return lookupGraphQuadsBatch(ctx, graphHd, filters, limitPerFilter, collector)
}

// lookupGraphQuadsBatch preserves filter order and applies limits independently.
func lookupGraphQuadsBatch(ctx context.Context, h world.CayleyHandle, filters []world.GraphQuad, limitPerFilter uint32, collector graphQuadBatchCollector) ([][]world.GraphQuad, error) {
	cfilters := make([]quad.Quad, len(filters))
	for i, filter := range filters {
		if filter == nil {
			filter = world.NewGraphQuad("", "", "", "")
		}
		cq, err := world.GraphQuadToCayleyQuad(filter, false)
		if err != nil {
			return nil, err
		}
		cfilters[i] = cq
	}

	// A single filter uses the native transaction; batches share cached iterator
	// and name lookups across filters.
	var cresults [][]quad.Quad
	var err error
	if collector != nil && len(cfilters) == 1 {
		cresults, err = collector.CollectFilteredQuadsBatch(ctx, cfilters, limitPerFilter)
	} else {
		cresults, err = world.CollectFilteredFullQuadsBatch(ctx, h, cfilters, limitPerFilter)
	}
	if err != nil {
		return nil, err
	}
	results := make([][]world.GraphQuad, len(cresults))
	for i, cquads := range cresults {
		results[i] = make([]world.GraphQuad, len(cquads))
		for j, cq := range cquads {
			results[i][j] = world.CayleyQuadToGraphQuad(cq)
		}
	}
	return results, nil
}

// queryGraphPathOnWorld retains one storage read scope for the traversal.
func (t *WorldState) queryGraphPathOnWorld(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	if t.discarded.Load() {
		return nil, tx.ErrDiscarded
	}
	if !t.write && t.store != nil {
		store, release, err := t.store.BeginReadOperation(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
		ctx = block.WithReadOperationStore(ctx, store)
	}
	return t.queryGraphPath(ctx, t.graphHd, query)
}

// queryGraphPath shares cached graph lookups throughout a bounded traversal.
func (t *WorldState) queryGraphPath(ctx context.Context, graphHd world.CayleyHandle, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	graphHd = world.NewReadOperationCayleyHandle(graphHd)
	graph := &graphPathReadOperation{
		WorldState: t,
		graphHd:    graphHd,
	}
	return world.QueryGraphPathWithLookups(ctx, graph, query)
}

// AccessCayleyGraph lends the scoped handle for read callbacks.
func (g *graphPathReadOperation) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	if write {
		return g.WorldState.AccessCayleyGraph(ctx, write, cb)
	}
	return cb(ctx, g.graphHd)
}

// LookupGraphQuads resolves one filter using the scoped handle.
func (g *graphPathReadOperation) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	filters := [1]world.GraphQuad{filter}
	results, err := g.LookupGraphQuadsBatch(ctx, filters[:], limit)
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// LookupGraphQuadsBatch shares the scoped handle across filters.
func (g *graphPathReadOperation) LookupGraphQuadsBatch(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	collector, _ := g.WorldState.graphHd.QuadStore.(graphQuadBatchCollector)
	return lookupGraphQuadsBatch(ctx, g.graphHd, filters, limitPerFilter, collector)
}

// QueryGraphPath continues traversal within the existing read operation.
func (g *graphPathReadOperation) QueryGraphPath(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	return world.QueryGraphPathWithLookups(ctx, g, query)
}

// setGraphQuad validates endpoints and records newly inserted relationships.
func (t *WorldState) setGraphQuad(ctx context.Context, q world.GraphQuad) error {
	err := t.InsertGraphQuads(ctx, []world.GraphQuad{q})
	if err != nil && graph.IsQuadExist(err) {
		return nil
	}
	return err
}

// deleteGraphQuadEntry validates a requested relationship before deletion.
func (t *WorldState) deleteGraphQuadEntry(ctx context.Context, q world.GraphQuad) error {
	return t.deleteGraphQuad(ctx, q, true)
}

// deleteGraphQuad removes a relationship and records the change when present.
func (t *WorldState) deleteGraphQuad(ctx context.Context, q world.GraphQuad, validate bool) error {
	if q == nil {
		return world.ErrNilQuad
	}
	if !t.write {
		return tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return tx.ErrDiscarded
	}

	subjKey := q.GetSubject()
	subj, subjFound, err := t.getObject(ctx, subjKey)
	if err != nil {
		return err
	}
	if subjFound {
		_, err = subj.incrementRev(ctx, false)
		if err != nil {
			return err
		}
	}

	objKey := q.GetObj()
	obj, objFound, err := t.getObject(ctx, objKey)
	if err != nil {
		return err
	}
	if objFound {
		_, err = obj.incrementRev(ctx, false)
		if err != nil {
			return err
		}
	}

	cq, err := world.GraphQuadToCayleyQuad(q, validate)
	if err != nil {
		return err
	}

	// Returns ErrQuadNotExist if not exists.
	err = t.graphHd.RemoveQuad(ctx, cq)
	if err != nil {
		if graph.IsQuadNotExist(err) {
			return nil
		}
		return err
	}

	// Record the removed relationship.
	_, err = t.queueWorldChange(ctx, &WorldChange{
		ChangeType: WorldChangeType_WorldChange_GRAPH_DELETE,
		Quad:       world.GraphQuadToQuad(q),
	})

	return err
}

// deleteGraphObject removes outgoing and incoming relationships for an object.
func (t *WorldState) deleteGraphObject(ctx context.Context, objKey string) error {
	if !t.write {
		return tx.ErrNotWrite
	}
	if objKey == "" {
		return nil
	}
	if t.discarded.Load() {
		return tx.ErrDiscarded
	}

	valueStr := world.KeyToGraphValue(objKey).String()

	// Collect outgoing and incoming relationships before deleting shared nodes.
	subjQuads, err := t.LookupGraphQuads(ctx, world.NewGraphQuad(valueStr, "", "", ""), 0)
	if err != nil {
		return err
	}

	objQuads, err := t.LookupGraphQuads(ctx, world.NewGraphQuad("", "", valueStr, ""), 0)
	if err != nil {
		return err
	}

	if len(subjQuads) == 0 && len(objQuads) == 0 {
		return nil
	}

	// Delete each quad individually via DeleteGraphQuad which handles
	// ErrQuadNotExist gracefully. Using RemoveNode here is unsafe: it
	// interleaves reading and deleting across direction passes, and
	// decNodes in one pass can delete shared node log entries that
	// subsequent passes need to resolve quads.
	for _, q := range subjQuads {
		if err := t.deleteGraphQuad(ctx, q, false); err != nil {
			return err
		}
	}
	for _, q := range objQuads {
		if err := t.deleteGraphQuad(ctx, q, false); err != nil {
			return err
		}
	}

	return nil
}

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
func (t *WorldState) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	return t.accessCayleyGraph(ctx, write, cb)
}

// LookupGraphQuadsBatch searches for graph quads for each filter in one graph read.
func (t *WorldState) LookupGraphQuadsBatch(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	return t.lookupGraphQuadsBatchOnWorld(ctx, filters, limitPerFilter)
}

// QueryGraphPath executes a bounded graph traversal.
func (t *WorldState) QueryGraphPath(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	return t.queryGraphPathOnWorld(ctx, query)
}

// SetGraphQuad sets a quad in the graph store.
func (t *WorldState) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	return t.setGraphQuad(ctx, q)
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (t *WorldState) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	return t.deleteGraphQuadEntry(ctx, q)
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
func (t *WorldState) DeleteGraphObject(ctx context.Context, objKey string) error {
	return t.deleteGraphObject(ctx, objKey)
}

// _ verifies the World graph contracts.
var (
	_ world.WorldStateGraph = (*WorldState)(nil)
	_ world.WorldStateGraph = (*graphPathReadOperation)(nil)
)
