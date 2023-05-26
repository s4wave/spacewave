package world_block

import (
	"context"

	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (t *Tx) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	if t.discarded {
		return tx.ErrDiscarded
	}

	return t.state.AccessCayleyGraph(ctx, write, cb)
}

// LookupGraphQuads searches for graph quads in the store.
func (t *Tx) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()

	if t.discarded {
		return nil, tx.ErrDiscarded
	}

	return t.state.LookupGraphQuads(ctx, filter, limit)
}

// SetGraphQuad sets a quad in the graph store.
// Subject: must be an existing object IRI: <object-key>
// Predicate: a predicate string, e.x. IRI: <ref>
// Object: an existing object IRI: <object-key>
// If already exists, returns nil.
func (t *Tx) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	t.rmtx.Lock()
	defer t.rmtx.Unlock()

	return t.state.SetGraphQuad(ctx, q)
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (t *Tx) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	t.rmtx.Lock()
	defer t.rmtx.Unlock()

	return t.state.DeleteGraphQuad(ctx, q)
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// May also remove objects with <predicate> or <value> set to the value.
func (t *Tx) DeleteGraphObject(ctx context.Context, value string) error {
	t.rmtx.Lock()
	defer t.rmtx.Unlock()

	return t.state.DeleteGraphObject(ctx, value)
}

// _ is a type assertion
var _ world.WorldStateGraph = ((*Tx)(nil))
