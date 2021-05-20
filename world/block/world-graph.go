package world_block

import (
	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/quad"
)

// SetGraphQuad sets a quad in the graph store.
// Subject: must be an existing object IRI: <object-id>
// Predicate: a predicate string, e.x. IRI: <ref>
// Object: an existing object IRI: <object-id>
// If already exists, returns nil.
// Checks the quad fields internally.
func (t *WorldState) SetGraphQuad(q world.GraphQuad) error {
	cq, err := world.GraphQuadToCayleyQuad(q, true)
	if err != nil {
		return err
	}
	return t.graphHd.AddQuad(cq)
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (t *WorldState) DeleteGraphQuad(q world.GraphQuad) error {
	if q == nil {
		return world.ErrNilQuad
	}
	cq, err := world.GraphQuadToCayleyQuad(q, true)
	if err != nil {
		return err
	}
	return t.graphHd.RemoveQuad(cq)
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// May also remove objects with <predicate> or <value> set to the value.
func (t *WorldState) DeleteGraphObject(value string) error {
	v := world.GraphQuadStringToCayleyValue(value)
	if _, ok := v.(quad.IRI); !ok {
		return world.ErrQuadSubjectNotIRI
	}
	return t.graphHd.RemoveNode(v)
}

// _ is a type assertion
var _ world.WorldStateGraph = ((*WorldState)(nil))
