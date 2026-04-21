package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
// All accesses of the handle should complete before returning cb.
// Try to make access (queries) as short as possible.
// Write operations will fail if the store is read-only.
func (t *Tx) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	unlock, err := t.rmtx.Lock(ctx, false)
	if err != nil {
		return err
	}
	defer unlock()

	return t.state.AccessCayleyGraph(ctx, write, cb)
}

// LookupGraphQuads searches for graph quads in the store.
func (t *Tx) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	unlock, err := t.rmtx.Lock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer unlock()

	return t.state.LookupGraphQuads(ctx, filter, limit)
}

// SetGraphQuad sets a quad in the graph store.
// Subject: must be an existing object IRI: <object-key>
// Predicate: a predicate string, e.x. IRI: <ref>
// Object: an existing object IRI: <object-key>
// If already exists, returns nil.
func (t *Tx) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	unlock, err := t.rmtx.Lock(ctx, true)
	if err != nil {
		return err
	}
	defer unlock()

	return t.state.SetGraphQuad(ctx, q)
}

// DeleteGraphQuad deletes a quad from the graph store.
// Note: if quad did not exist, returns nil.
func (t *Tx) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	unlock, err := t.rmtx.Lock(ctx, true)
	if err != nil {
		return err
	}
	defer unlock()

	return t.state.DeleteGraphQuad(ctx, q)
}

// DeleteGraphObject deletes all quads with Subject or Object set to value.
// May also remove objects with <predicate> or <value> set to the value.
func (t *Tx) DeleteGraphObject(ctx context.Context, value string) error {
	unlock, err := t.rmtx.Lock(ctx, true)
	if err != nil {
		return err
	}
	defer unlock()

	return t.state.DeleteGraphObject(ctx, value)
}

// _ is a type assertion
var _ world.WorldStateGraph = ((*Tx)(nil))
