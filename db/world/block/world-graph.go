package world_block

import (
	"context"
	"io"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (t *WorldState) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	if t.discarded.Load() {
		return tx.ErrDiscarded
	}

	hd := t.graphHd
	// TODO TODO: wrap the graph handle to update the changelog if writes are applied here.
	return cb(ctx, hd)
}

// LookupGraphQuads searches for graph quads in the store.
func (t *WorldState) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	if t.discarded.Load() {
		return nil, tx.ErrDiscarded
	}

	// Treat nil filter as empty filter (matches all quads)
	if filter == nil {
		filter = world.NewGraphQuad("", "", "", "")
	}

	cq, err := world.GraphQuadToCayleyQuad(filter, false)
	if err != nil {
		return nil, err
	}

	var quads []world.GraphQuad
	err = world.FilterIterateQuads(ctx, t.graphHd, cq, func(q quad.Quad) error {
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

// SetGraphQuad sets a quad in the graph store.
// If already exists, returns nil.
func (t *WorldState) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
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

	// check if already exists
	ex, err := world.CheckQuadExists(ctx, t.graphHd, cq)
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
func (t *WorldState) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
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
func (t *WorldState) DeleteGraphObject(ctx context.Context, objKey string) error {
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

// _ is a type assertion
var _ world.WorldStateGraph = ((*WorldState)(nil))
