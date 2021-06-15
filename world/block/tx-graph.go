package world_block

import (
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// LookupGraphQuad checks if a graph quad exists in the store.
// If not found, returns false, nil.
func (t *Tx) LookupGraphQuad(q world.GraphQuad) (bool, error) {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	if t.discarded {
		return false, tx.ErrDiscarded
	}

	return t.state.LookupGraphQuad(q)
}

// SetGraphQuad sets a quad in the graph store.
// Subject: must be an existing object IRI: <object-id>
// Predicate: a predicate string, e.x. IRI: <ref>
// Object: an existing object IRI: <object-id>
// If already exists, returns nil.
func (t *Tx) SetGraphQuad(q world.GraphQuad) error {
	t.rmtx.Lock()
	defer t.rmtx.Unlock()

	if t.discarded {
		return tx.ErrDiscarded
	}

	return t.state.SetGraphQuad(q)
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (t *Tx) DeleteGraphQuad(q world.GraphQuad) error {
	t.rmtx.Lock()
	defer t.rmtx.Unlock()

	if t.discarded {
		return tx.ErrDiscarded
	}

	return t.state.DeleteGraphQuad(q)
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// May also remove objects with <predicate> or <value> set to the value.
// Returns number of removed quads and any error.
func (t *Tx) DeleteGraphObject(value string) error {
	t.rmtx.Lock()
	defer t.rmtx.Unlock()

	if t.discarded {
		return tx.ErrDiscarded
	}

	return t.state.DeleteGraphObject(value)
}

// _ is a type assertion
var _ world.WorldStateGraph = ((*Tx)(nil))
