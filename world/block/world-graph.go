package world_block

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/quad"
)

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (t *WorldState) AccessCayleyGraph(ctx context.Context, write bool, cb func(h world.CayleyHandle) error) error {
	hd := t.graphHd
	// TODO TODO: wrap the graph handle to update the changelog if writes are applied here.
	return cb(hd)
}

// LookupGraphQuads searches for graph quads in the store.
func (t *WorldState) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	cq, err := world.GraphQuadToCayleyQuad(filter, false)
	if err != nil {
		return nil, err
	}

	var quads []world.GraphQuad
	err = world.FilterIterateQuads(ctx, t.graphHd, cq, func(q quad.Quad) error {
		quads = append(quads, world.CayleyQuadToGraphQuad(q))
		if limit != 0 && uint32(len(quads)) >= limit {
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
	subjRef, err := world.MustGetObject(ctx, t, subjKey)
	if err != nil {
		return err
	}

	objKey, err := world.GraphValueToKey(q.GetObj())
	if err != nil {
		return err
	}
	objRef, err := world.MustGetObject(ctx, t, objKey)
	if err != nil {
		return err
	}

	// add quad
	err = t.graphHd.AddQuad(cq)
	if err != nil {
		return err
	}

	// increment rev # on the affected objects
	// note: does not add INCREMENT_REV to changelog
	_, err = subjRef.(*ObjectState).incrementRev(ctx, false)
	if err != nil {
		return err
	}
	_, err = objRef.(*ObjectState).incrementRev(ctx, false)
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
	if q == nil {
		return world.ErrNilQuad
	}
	if !t.write {
		return tx.ErrNotWrite
	}

	subjKey := q.GetSubject()
	subj, subjFound, err := t.GetObject(ctx, subjKey)
	if err != nil {
		return err
	}
	if subjFound {
		_, err = subj.(*ObjectState).incrementRev(ctx, false)
		if err != nil {
			return err
		}
	}

	objKey := q.GetObj()
	obj, objFound, err := t.GetObject(ctx, objKey)
	if err != nil {
		return err
	}
	if objFound {
		_, err = obj.(*ObjectState).incrementRev(ctx, false)
		if err != nil {
			return err
		}
	}

	cq, err := world.GraphQuadToCayleyQuad(q, true)
	if err != nil {
		return err
	}

	err = t.graphHd.RemoveQuad(cq)
	if err != nil {
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

	value := quad.IRI(objKey)
	valueStr := value.String()

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
	if len(subjQuads) == 0 || len(objQuads) == 0 {
		return nil
	}

	// clear all quads matching the node
	err = t.graphHd.RemoveNode(value)
	if err != nil {
		return err
	}

	// increment object revision
	objState, objStateFound, err := t.GetObject(ctx, objKey)
	if err != nil {
		return err
	}
	if objStateFound {
		_, err = objState.(*ObjectState).incrementRev(ctx, false)
		if err != nil {
			return err
		}
	}

	// update changelog
	queueDel := func(q world.GraphQuad) error {
		_, err := t.queueWorldChange(ctx, &WorldChange{
			ChangeType: WorldChangeType_WorldChange_GRAPH_DELETE,
			Quad:       world.GraphQuadToQuad(q),
		})
		return err
	}
	for _, subjQuad := range subjQuads {
		if err := queueDel(subjQuad); err != nil {
			return err
		}
	}
	for _, objQuad := range objQuads {
		if err := queueDel(objQuad); err != nil {
			return err
		}
	}

	return nil
}

// _ is a type assertion
var _ world.WorldStateGraph = ((*WorldState)(nil))
