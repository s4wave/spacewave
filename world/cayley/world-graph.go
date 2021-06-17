package world_cayley

import (
	"context"
	"io"
	"sync"

	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/quad"
)

// WorldStateGraph implements the world state graph interface with a Cayley
// graph handle.
type WorldStateGraph struct {
	// ctx is the context
	ctx context.Context
	// objHd is the object store handle
	objHd world.WorldStateObject
	// rmtx guards graphd
	rmtx sync.RWMutex
	// graphHd is the cayley graph handle
	graphHd *cayley.Handle
}

// NewWorldStateGraph constructs a new WorldStateGraph handle.
func NewWorldStateGraph(ctx context.Context, objHd world.WorldStateObject, graphHd *cayley.Handle) *WorldStateGraph {
	return &WorldStateGraph{
		ctx:     ctx,
		objHd:   objHd,
		graphHd: graphHd,
	}
}

// GetGraphHandle returns the graph handle.
func (t *WorldStateGraph) GetGraphHandle() world.CayleyHandle {
	t.rmtx.RLock()
	hd := t.graphHd
	t.rmtx.RUnlock()
	return hd
}

// SetGraphHandle sets an updated graph handle.
// Not concurrent safe with the operations.
// Handle must not be nil.
func (t *WorldStateGraph) SetGraphHandle(hd *cayley.Handle) {
	t.rmtx.Lock()
	t.graphHd = hd
	t.rmtx.Unlock()
}

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (t *WorldStateGraph) AccessCayleyGraph(write bool, cb func(h world.CayleyHandle) error) error {
	t.rmtx.RLock()
	hd := t.graphHd
	t.rmtx.RUnlock()
	return cb(hd)
}

// LookupGraphQuads searches for graph quads in the store.
func (t *WorldStateGraph) LookupGraphQuads(filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	cq, err := world.GraphQuadToCayleyQuad(filter, false)
	if err != nil {
		return nil, err
	}

	t.rmtx.RLock()
	defer t.rmtx.RUnlock()
	var quads []world.GraphQuad
	err = filterIterateQuads(t.ctx, t.graphHd, cq, func(q quad.Quad) error {
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
// Subject: must be an existing object IRI: <object-id>
// Predicate: a predicate string, e.x. IRI: <ref>
// Object: an existing object IRI: <object-id>
// If already exists, returns nil.
// Checks the quad fields internally.
func (t *WorldStateGraph) SetGraphQuad(q world.GraphQuad) error {
	cq, err := world.GraphQuadToCayleyQuad(q, true)
	if err != nil {
		return err
	}

	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	// check if already exists
	ex, err := checkQuadExists(t.ctx, t.graphHd, cq)
	if err != nil {
		return err
	}
	if ex {
		// already exists
		return nil
	}

	// get handles to the affected Objects

	// the ensureIsIRI already stripped the < > prefix / suffix
	subjIRI, err := world.GraphEnsureIsIRI(q.GetSubject())
	if err != nil {
		return err
	}
	subjKey := string(subjIRI)
	subjRef, err := world.MustGetObject(t.objHd, subjKey)
	if err != nil {
		return err
	}

	objIRI, err := world.GraphEnsureIsIRI(q.GetObject())
	if err != nil {
		return err
	}
	objKey := string(objIRI)
	objRef, err := world.MustGetObject(t.objHd, objKey)
	if err != nil {
		return err
	}

	// add quad
	err = t.graphHd.AddQuad(cq)
	if err != nil {
		return err
	}

	// increment revision on affected objects
	_, err = subjRef.IncrementRev()
	if err != nil {
		return err
	}
	_, err = objRef.IncrementRev()
	if err != nil {
		return err
	}
	return nil
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (t *WorldStateGraph) DeleteGraphQuad(q world.GraphQuad) error {
	if q == nil {
		return world.ErrNilQuad
	}
	cq, err := world.GraphQuadToCayleyQuad(q, true)
	if err != nil {
		return err
	}

	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	return t.graphHd.RemoveQuad(cq)
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// May also remove objects with <predicate> or <value> set to the value.
func (t *WorldStateGraph) DeleteGraphObject(value string) error {
	v := world.GraphQuadStringToCayleyValue(value)
	if _, ok := v.(quad.IRI); !ok {
		return world.ErrQuadSubjectNotIRI
	}

	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	return t.graphHd.RemoveNode(v)
}

// _ is a type assertion
var _ world.WorldStateGraph = ((*WorldStateGraph)(nil))
