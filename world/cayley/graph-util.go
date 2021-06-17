package world_cayley

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/cayley/query/shape"
	"github.com/cayleygraph/quad"
)

// CheckQuadExists checks if the quad exists on the graph handle.
func CheckQuadExists(ctx context.Context, h world.CayleyHandle, gq quad.Quad) (bool, error) {
	// there may be a faster way to lookup a quad
	var found bool
	err := FilterIterateQuads(ctx, h, gq, func(q quad.Quad) error {
		if q.IsValid() {
			found = true
			return io.EOF
		}
		return nil
	})
	if err == io.EOF {
		err = nil
	}
	return found, err
}

// FilterIterateQuads iterates over quads matching the input quad.
// empty fields are ignored
func FilterIterateQuads(ctx context.Context, h world.CayleyHandle, gq quad.Quad, cb func(q quad.Quad) error) error {
	var q shape.Quads
	subject := gq.Subject
	if subject != nil {
		q = append(q, shape.QuadFilter{Dir: quad.Subject, Values: shape.Lookup([]quad.Value{subject})})
	}
	pred := gq.Predicate
	if pred != nil {
		q = append(q, shape.QuadFilter{Dir: quad.Predicate, Values: shape.Lookup([]quad.Value{pred})})
	}
	obj := gq.Object
	if obj != nil {
		q = append(q, shape.QuadFilter{Dir: quad.Object, Values: shape.Lookup([]quad.Value{obj})})
	}
	val := gq.Label
	if val != nil {
		q = append(q, shape.QuadFilter{Dir: quad.Label, Values: shape.Lookup([]quad.Value{val})})
	}
	return OptimizeIterateQuads(ctx, h, q, cb)
}

// OptimizeIterateQuads optimizes a shape and iterates over the quads.
func OptimizeIterateQuads(ctx context.Context, h world.CayleyHandle, sh shape.Shape, cb func(q quad.Quad) error) error {
	sh, _ = shape.Optimize(ctx, sh, h)
	it := sh.BuildIterator(h).Iterate()
	defer it.Close()
	rsc := graph.NewResultReader(h, it)
	for {
		q, err := rsc.ReadQuad()
		if err == nil {
			err = cb(q)
		}
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return err
		}
	}
}
