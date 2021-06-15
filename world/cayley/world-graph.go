package world_cayley

import (
	"context"
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

// SetGraphHandle sets an updated graph handle.
// Not concurrent safe with the operations.
// Handle must not be nil.
func (t *WorldStateGraph) SetGraphHandle(hd *cayley.Handle) {
	t.rmtx.Lock()
	t.graphHd = hd
	t.rmtx.Unlock()
}

// LookupGraphQuad checks if a graph quad exists in the store.
// Filters based on subject.
// If the predicate, object, or value fields are empty, matches any.
// If not found, returns false, nil.
func (t *WorldStateGraph) LookupGraphQuad(q world.GraphQuad) (bool, error) {
	cq, err := world.GraphQuadToCayleyQuad(q, true)
	if err != nil {
		return false, err
	}

	t.rmtx.RLock()
	defer t.rmtx.RUnlock()
	return checkQuadExists(t.ctx, t.graphHd, cq)
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
	subjIRI, err := ensureIsIRI(q.GetSubject())
	if err != nil {
		return err
	}
	subjKey := string(subjIRI)
	subjRef, err := world.MustGetObject(t.objHd, subjKey)
	if err != nil {
		return err
	}

	objIRI, err := ensureIsIRI(q.GetObject())
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
