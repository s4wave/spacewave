package world_block

import (
	"io"

	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/quad"
)

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (t *WorldState) AccessCayleyGraph(write bool, cb func(h world.CayleyHandle) error) error {
	hd := t.graphHd
	// TODO: wrap the graph handle to update the changelog if writes are applied here.
	return cb(hd)
}

// LookupGraphQuads searches for graph quads in the store.
func (t *WorldState) LookupGraphQuads(filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	cq, err := world.GraphQuadToCayleyQuad(filter, false)
	if err != nil {
		return nil, err
	}

	var quads []world.GraphQuad
	err = world.FilterIterateQuads(t.ctx, t.graphHd, cq, func(q quad.Quad) error {
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
func (t *WorldState) SetGraphQuad(q world.GraphQuad) error {
	cq, err := world.GraphQuadToCayleyQuad(q, true)
	if err != nil {
		return err
	}

	// check if already exists
	ex, err := world.CheckQuadExists(t.ctx, t.graphHd, cq)
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
	subjRef, err := world.MustGetObject(t, subjKey)
	if err != nil {
		return err
	}

	objKey, err := world.GraphValueToKey(q.GetObj())
	if err != nil {
		return err
	}
	objRef, err := world.MustGetObject(t, objKey)
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
	_, err = subjRef.(*ObjectState).incrementRev(false)
	if err != nil {
		return err
	}
	_, err = objRef.(*ObjectState).incrementRev(false)
	if err != nil {
		return err
	}

	// update changelog with graph set
	_, err = t.queueWorldChange(&WorldChange{
		ChangeType: WorldChangeType_WorldChange_GRAPH_SET,
		Quad:       world.GraphQuadToQuad(q),
	})
	return err
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (t *WorldState) DeleteGraphQuad(q world.GraphQuad) error {
	if q == nil {
		return world.ErrNilQuad
	}

	subjKey := q.GetSubject()
	subj, subjFound, err := t.GetObject(subjKey)
	if err != nil {
		return err
	}
	if subjFound {
		_, err = subj.(*ObjectState).incrementRev(false)
		if err != nil {
			return err
		}
	}

	objKey := q.GetObj()
	obj, objFound, err := t.GetObject(objKey)
	if err != nil {
		return err
	}
	if objFound {
		_, err = obj.(*ObjectState).incrementRev(false)
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
	_, err = t.queueWorldChange(&WorldChange{
		ChangeType: WorldChangeType_WorldChange_GRAPH_DELETE,
		Quad:       world.GraphQuadToQuad(q),
	})
	return err
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
func (t *WorldState) DeleteGraphObject(objKey string) error {
	value := quad.IRI(objKey)
	valueStr := value.String()

	// find all matching quads where subject == value
	subjQuads, err := t.LookupGraphQuads(world.NewGraphQuad(valueStr, "", "", ""), 0)
	if err != nil {
		return err
	}

	// find all matching quads where object == value
	objQuads, err := t.LookupGraphQuads(world.NewGraphQuad("", "", valueStr, ""), 0)
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
	objState, objStateFound, err := t.GetObject(objKey)
	if err != nil {
		return err
	}
	if objStateFound {
		_, err = objState.(*ObjectState).incrementRev(false)
		if err != nil {
			return err
		}
	}

	// update changelog
	queueDel := func(q world.GraphQuad) error {
		_, err := t.queueWorldChange(&WorldChange{
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
