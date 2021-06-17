package world_cayley

import (
	"context"
	"io"

	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/cayley/query/shape"
	"github.com/cayleygraph/quad"
)

// checkQuadExists checks if the quad exists on the graph handle.
func checkQuadExists(ctx context.Context, h *cayley.Handle, gq quad.Quad) (bool, error) {
	// there may be a faster way to lookup a quad
	var found bool
	err := filterIterateQuads(ctx, h, gq, func(q quad.Quad) error {
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

// filterIterateQuads iterates over quads matching the input quad.
// empty fields are ignored
func filterIterateQuads(ctx context.Context, h *cayley.Handle, gq quad.Quad, cb func(q quad.Quad) error) error {
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
	return optimizeIterateQuads(ctx, h, q, cb)
}

// optimizeIterateQuads optimizes a shape and iterates over the quads.
func optimizeIterateQuads(ctx context.Context, h *cayley.Handle, sh shape.Shape, cb func(q quad.Quad) error) error {
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
