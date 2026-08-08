package world_block

import (
	"context"
	"io"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (t *WorldState) accessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	if t.discarded.Load() {
		return tx.ErrDiscarded
	}

	hd := t.graphHd
	// TODO TODO: wrap the graph handle to update the changelog if writes are applied here.
	return cb(ctx, hd)
}

// LookupGraphQuads searches for graph quads in the store.
func (t *WorldState) lookupGraphQuadsOnWorld(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
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

	graphHd := world.NewReadOperationCayleyHandle(t.graphHd)
	return lookupGraphQuads(ctx, graphHd, filter, limit)
}

// LookupGraphQuadsBatch searches for graph quads for each filter in one graph read.
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

	graphHd := world.NewReadOperationCayleyHandle(t.graphHd)
	return lookupGraphQuadsBatch(ctx, graphHd, filters, limitPerFilter)
}

func lookupGraphQuads(ctx context.Context, h world.CayleyHandle, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	// Treat nil filter as empty filter (matches all quads)
	if filter == nil {
		filter = world.NewGraphQuad("", "", "", "")
	}

	cq, err := world.GraphQuadToCayleyQuad(filter, false)
	if err != nil {
		return nil, err
	}

	var quads []world.GraphQuad
	err = world.FilterIterateQuads(ctx, h, cq, func(q quad.Quad) error {
		quads = append(quads, world.CayleyQuadToGraphQuad(q))
		if limit != 0 && uint32(len(quads)) >= limit { //nolint:gosec
			return io.EOF
		}
		return nil
	})
	if err == io.EOF {
		err = nil
	}
	return quads, err
}

func lookupGraphQuadsBatch(ctx context.Context, h world.CayleyHandle, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
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
	cresults, err := world.CollectFilteredFullQuadsBatch(ctx, h, cfilters, limitPerFilter)
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

type graphQuadBatchCollector interface {
	CollectFilteredQuadsBatch(ctx context.Context, filters []quad.Quad, limitPerFilter uint32) ([][]quad.Quad, error)
}

func (t *WorldState) graphQuadExists(ctx context.Context, cq quad.Quad) (bool, error) {
	if collector, ok := t.graphHd.QuadStore.(graphQuadBatchCollector); ok {
		filters := [1]quad.Quad{cq}
		results, err := collector.CollectFilteredQuadsBatch(ctx, filters[:], 1)
		if err != nil {
			return false, err
		}
		if len(results) == 0 {
			return false, nil
		}
		for _, q := range results[0] {
			if q.IsValid() && world.QuadEqual(q, cq) {
				return true, nil
			}
		}
		return false, nil
	}
	return world.CheckQuadExists(ctx, t.graphHd, cq)
}

// QueryGraphPath executes a bounded graph traversal.
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

func (t *WorldState) queryGraphPath(ctx context.Context, graphHd world.CayleyHandle, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	graphHd = world.NewReadOperationCayleyHandle(graphHd)
	graph := &graphPathReadOperation{
		WorldState: t,
		graphHd:    graphHd,
	}
	return world.QueryGraphPathWithLookups(ctx, graph, query)
}

type graphPathReadOperation struct {
	*WorldState

	graphHd world.CayleyHandle
}

func (g *graphPathReadOperation) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	if write {
		return g.WorldState.AccessCayleyGraph(ctx, write, cb)
	}
	return cb(ctx, g.graphHd)
}

func (g *graphPathReadOperation) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	return lookupGraphQuads(ctx, g.graphHd, filter, limit)
}

func (g *graphPathReadOperation) LookupGraphQuadsBatch(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	return lookupGraphQuadsBatch(ctx, g.graphHd, filters, limitPerFilter)
}

func (g *graphPathReadOperation) QueryGraphPath(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	return world.QueryGraphPathWithLookups(ctx, g, query)
}

// SetGraphQuad sets a quad in the graph store.
// If already exists, returns nil.
func (t *WorldState) setGraphQuad(ctx context.Context, q world.GraphQuad) error {
	if !t.write {
		return tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return tx.ErrDiscarded
	}

	cq, err := world.GraphQuadToCayleyQuad(q, true)
	if err != nil {
		return err
	}

	ex, err := t.graphQuadExists(ctx, cq)
	if err != nil {
		return err
	}
	if ex {
		// already exists
		return nil
	}

	// the ensureIsIRI already stripped the < > prefix / suffix
	subjKey, err := world.GraphValueToKey(q.GetSubject())
	if err != nil {
		return err
	}
	subjRef, err := t.mustGetObject(ctx, subjKey)
	if err != nil {
		return err
	}

	objKey, err := world.GraphValueToKey(q.GetObj())
	if err != nil {
		return err
	}
	objRef, err := t.mustGetObject(ctx, objKey)
	if err != nil {
		return err
	}

	// add quad
	err = t.graphHd.AddQuad(ctx, cq)
	if err != nil {
		return err
	}

	// increment rev # on the affected objects
	// note: does not add INCREMENT_REV to changelog
	_, err = subjRef.incrementRev(ctx, false)
	if err != nil {
		return err
	}
	_, err = objRef.incrementRev(ctx, false)
	if err != nil {
		return err
	}

	// update changelog with graph set
	_, err = t.queueWorldChange(ctx, &WorldChange{
		ChangeType: WorldChangeType_WorldChange_GRAPH_SET,
		Quad:       world.GraphQuadToQuad(q),
	})
	return err
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (t *WorldState) deleteGraphQuadEntry(ctx context.Context, q world.GraphQuad) error {
	return t.deleteGraphQuad(ctx, q, true)
}

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

	// update changelog
	_, err = t.queueWorldChange(ctx, &WorldChange{
		ChangeType: WorldChangeType_WorldChange_GRAPH_DELETE,
		Quad:       world.GraphQuadToQuad(q),
	})

	return err
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
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

	// find all matching quads where subject == value
	subjQuads, err := t.LookupGraphQuads(ctx, world.NewGraphQuad(valueStr, "", "", ""), 0)
	if err != nil {
		return err
	}

	// find all matching quads where object == value
	objQuads, err := t.LookupGraphQuads(ctx, world.NewGraphQuad("", "", valueStr, ""), 0)
	if err != nil {
		return err
	}

	// if no matches, stop here.
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

// _ is a type assertion
var (
	_ world.WorldStateGraph = (*WorldState)(nil)
	_ world.WorldStateGraph = (*graphPathReadOperation)(nil)
)
