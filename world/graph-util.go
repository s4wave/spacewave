package world

import (
	"context"
	"io"
	"slices"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/query/shape"
)

// QuadEqual checks if two quads are equal.
func QuadEqual(q1, q2 quad.Quad) bool {
	// TODO faster check
	return q1.String() == q2.String()
}

// CheckQuadExists checks if the quad exists on the graph handle.
func CheckQuadExists(ctx context.Context, h CayleyHandle, gq quad.Quad) (bool, error) {
	// there may be a faster way to lookup a quad
	var found bool
	err := FilterIterateQuads(ctx, h, gq, func(q quad.Quad) error {
		if q.IsValid() && QuadEqual(q, gq) {
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
func FilterIterateQuads(ctx context.Context, h CayleyHandle, gq quad.Quad, cb func(q quad.Quad) error) error {
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
func OptimizeIterateQuads(ctx context.Context, h CayleyHandle, sh shape.Shape, cb func(q quad.Quad) error) error {
	sh, _, err := shape.Optimize(ctx, sh, h)
	if err != nil {
		return err
	}
	it := sh.BuildIterator(ctx, h).Iterate(ctx)
	defer it.Close()
	rsc := graph.NewResultReader(ctx, h, it)
	for {
		q, err := rsc.ReadQuad(ctx)
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

// IteratePathWithKeys starts & iterates a path from the given object keys.
func IteratePathWithKeys(
	ctx context.Context,
	ws WorldStateGraph,
	entityKeys []string,
	pathCb func(p *cayley.Path) (*cayley.Path, error),
	valueCb func(objKey string) (ctnu bool, err error),
) error {
	if valueCb == nil {
		return nil
	}

	gv := make([]quad.Value, len(entityKeys))
	for i, ek := range entityKeys {
		gv[i] = KeyToGraphValue(ek)
	}

	return ws.AccessCayleyGraph(ctx, false, func(ctx context.Context, h CayleyHandle) error {
		p := cayley.StartPath(h, gv...)
		if pathCb != nil {
			var err error
			p, err = pathCb(p)
			if err != nil || p == nil {
				return err
			}
		}

		it := p.BuildIterator(ctx).Iterate(ctx)
		defer it.Close()
		for it.Next(ctx) {
			res, err := it.Result(ctx)
			if err != nil {
				return err
			}
			qv, err := h.NameOf(ctx, res)
			if err != nil {
				return err
			}
			key, err := QuadValueToKey(qv)
			if err != nil {
				return err
			}
			ctnu, err := valueCb(key)
			if err != nil || !ctnu {
				return err
			}
		}
		return it.Err()
	})
}

// CollectPathWithKeys collects the object keys for a given path.
func CollectPathWithKeys(
	ctx context.Context,
	ws WorldStateGraph,
	entityKeys []string,
	pathCb func(p *cayley.Path) (*cayley.Path, error),
) ([]string, error) {
	var output []string
	seen := make(map[string]struct{})
	err := IteratePathWithKeys(
		ctx,
		ws,
		entityKeys,
		pathCb,
		func(objKey string) (ctnu bool, err error) {
			if _, ok := seen[objKey]; !ok {
				seen[objKey] = struct{}{}
				output = append(output, objKey)
			}
			return true, nil
		},
	)
	slices.Sort(output)
	return output, err
}
