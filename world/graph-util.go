package world

import (
	"context"
	"io"
	"slices"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/graph/iterator"
	"github.com/aperturerobotics/cayley/graph/refs"
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
// Empty fields are ignored.
func FilterIterateQuads(ctx context.Context, h CayleyHandle, gq quad.Quad, cb func(q quad.Quad) error) error {
	return IterateFilteredFullQuads(ctx, h, gq, cb)
}

// IterateFilteredFullQuads iterates over full quads matching a concrete quad filter.
func IterateFilteredFullQuads(ctx context.Context, h CayleyHandle, filter quad.Quad, cb func(q quad.Quad) error) error {
	if cb == nil {
		return nil
	}
	if !hasQuadFilter(filter) {
		it := h.QuadsAllIterator(ctx).Iterate(ctx)
		defer it.Close()
		return iterateQuadResults(ctx, h, it, cb)
	}

	dir, ref, ok, err := selectQuadFilterIterator(ctx, h, filter)
	if err != nil || !ok {
		return err
	}

	it := h.QuadIterator(ctx, dir, ref).Iterate(ctx)
	defer it.Close()
	return iterateQuadResults(ctx, h, it, func(q quad.Quad) error {
		if !quadMatchesFilter(q, filter) {
			return nil
		}
		return cb(q)
	})
}

// IterateFullQuads iterates over the full quads matched by a shape.
func IterateFullQuads(ctx context.Context, h CayleyHandle, sh shape.Shape, cb func(q quad.Quad) error) error {
	if cb == nil {
		return nil
	}

	// Do not call shape.Optimize here. Optimized shapes may yield node/value refs
	// instead of quad refs, but graph.NewResultReader requires each iterator result
	// to be a quad ref so QuadStore.Quad can recover all four directions.
	it := sh.BuildIterator(ctx, h).Iterate(ctx)
	defer it.Close()
	return iterateQuadResults(ctx, h, it, cb)
}

func iterateQuadResults(ctx context.Context, h CayleyHandle, it iterator.Scanner, cb func(q quad.Quad) error) error {
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

func selectQuadFilterIterator(ctx context.Context, h CayleyHandle, filter quad.Quad) (quad.Direction, graph.Ref, bool, error) {
	var bestDir quad.Direction
	var bestRef graph.Ref
	var bestSize refs.Size
	var found bool
	for _, dir := range quad.Directions {
		val := filter.Get(dir)
		if val == nil {
			continue
		}
		ref, err := h.ValueOf(ctx, val)
		if err != nil || ref == nil {
			return 0, nil, false, err
		}
		size, err := h.QuadIteratorSize(ctx, dir, ref)
		if err != nil {
			return 0, nil, false, err
		}
		if !found || size.Value < bestSize.Value {
			bestDir = dir
			bestRef = ref
			bestSize = size
			found = true
		}
	}
	return bestDir, bestRef, found, nil
}

func hasQuadFilter(q quad.Quad) bool {
	for _, dir := range quad.Directions {
		if q.Get(dir) != nil {
			return true
		}
	}
	return false
}

func quadMatchesFilter(q, filter quad.Quad) bool {
	for _, dir := range quad.Directions {
		val := filter.Get(dir)
		if val == nil {
			continue
		}
		if !quadValuesEqual(q.Get(dir), val) {
			return false
		}
	}
	return true
}

func quadValuesEqual(v1, v2 quad.Value) bool {
	if v1 == nil || v2 == nil {
		return v1 == v2
	}
	return v1.String() == v2.String()
}

// IteratePathWithKeys starts & iterates a path from the given object keys.
func IteratePathWithKeys(
	ctx context.Context,
	ws WorldStateGraph,
	entityKeys []string,
	pathCb func(p *cayley.Path) (*cayley.Path, error),
	valueCb func(objKey string) (ctnu bool, err error),
) error {
	if valueCb == nil || len(entityKeys) == 0 {
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

// CollectPathWithKeys collects the object keys for a given path starting at entityKeys.
//
// If the entityKeys list is empty, returns nil, nil.
func CollectPathWithKeys(
	ctx context.Context,
	ws WorldStateGraph,
	entityKeys []string,
	pathCb func(p *cayley.Path) (*cayley.Path, error),
) ([]string, error) {
	if len(entityKeys) == 0 {
		return nil, nil
	}

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
