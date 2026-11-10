package kvtx_cayley_nodegraph

import (
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/graph/iterator"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/query/shape"
)

// nameCache resolves each node's value once.
type nameCache struct {
	qs     *QuadStore
	values map[uint64]quad.Value
}

// newNameCache returns an empty cache.
func newNameCache(qs *QuadStore) *nameCache {
	return &nameCache{qs: qs, values: make(map[uint64]quad.Value)}
}

// name returns the value of a node ID.
func (c *nameCache) name(ctx context.Context, id uint64) (quad.Value, error) {
	// A name read once is cached for the call.
	if v, ok := c.values[id]; ok {
		return v, nil
	}

	// Read it from the node's meta otherwise.
	v, err := c.qs.nameOfID(ctx, id)
	if err != nil {
		return nil, err
	}
	c.values[id] = v
	return v, nil
}

// quad returns the values of a quad's IDs.
func (c *nameCache) quad(ctx context.Context, q quadRef) (quad.Quad, error) {
	var out quad.Quad
	for _, d := range quad.Directions {
		id := q.dir(d)
		if id == 0 {
			continue
		}
		v, err := c.name(ctx, id)
		if err != nil {
			return quad.Quad{}, err
		}
		out.Set(d, v)
	}
	return out, nil
}

// resolveFilter returns the IDs of a filter's values and seeds the cache with
// them, with ok false when a value has no node and so nothing matches.
func (c *nameCache) resolveFilter(ctx context.Context, f quad.Quad) (quadRef, bool, error) {
	var ids [4]uint64
	for i, d := range quad.Directions {
		v := f.Get(d)
		if v == nil {
			continue
		}
		id, err := c.qs.lookupID(ctx, v.String())
		if err != nil || id == 0 {
			return quadRef{}, false, err
		}
		ids[i] = id
		c.values[id] = v
	}
	return quadRef{s: ids[0], p: ids[1], o: ids[2], l: ids[3]}, true, nil
}

// bestRange returns the range of the quads matching f. With both endpoints
// and no predicate set, it walks the smaller endpoint's range.
func (qs *QuadStore) bestRange(ctx context.Context, f quadRef) (edgeRange, error) {
	// Only a filter on both endpoints and no predicate has a choice.
	r := rangeOf(f)
	if f.s == 0 || f.o == 0 || f.p != 0 {
		return r, nil
	}

	// Walk the object's in range when its bounded probe is smaller.
	in := edgeRange{node: f.o, tag: tagIn, filter: f}
	outSize, err := qs.probeSize(ctx, r)
	if err != nil {
		return r, err
	}
	inSize, err := qs.probeSize(ctx, in)
	if err != nil || inSize.Value >= outSize.Value {
		return r, err
	}
	return in, nil
}

// CollectFilteredQuadsBatch returns the quads matching each filter, at most
// limitPerFilter each when it is set. A filter's unset directions match any
// value; a filter with no direction set returns every quad.
func (qs *QuadStore) CollectFilteredQuadsBatch(ctx context.Context, filters []quad.Quad, limitPerFilter uint32) ([][]quad.Quad, error) {
	names := newNameCache(qs)
	results := make([][]quad.Quad, len(filters))
	for i, f := range filters {
		ids, ok, err := names.resolveFilter(ctx, f)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		r, err := qs.bestRange(ctx, ids)
		if err != nil {
			return nil, err
		}
		err = qs.walk(ctx, r, nil, func(q quadRef) (bool, error) {
			cq, err := names.quad(ctx, q)
			if err != nil {
				return false, err
			}
			results[i] = append(results[i], cq)
			return limitPerFilter == 0 || uint32(len(results[i])) < limitPerFilter, nil //nolint:gosec
		})
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

// IterateSubjectsAfter calls cb with each distinct subject of the quads with
// obj and pred, in ID order, after the subject after, or from the first when
// after is nil.
func (qs *QuadStore) IterateSubjectsAfter(ctx context.Context, obj, pred, after graph.Ref, cb func(subject graph.Ref) error) error {
	// Refs from another store name nothing here.
	o, ok := asNodeID(obj)
	if !ok {
		return nil
	}
	p, ok := asNodeID(pred)
	if !ok {
		return nil
	}

	// Walk the object's in range for pred, from just past after.
	r := edgeRange{node: o, tag: tagIn, narrow: []uint64{p}, filter: quadRef{p: p, o: o}}
	var start []byte
	var last uint64
	if a, ok := asNodeID(after); ok {
		start, last = rangeKey(o, tagIn, p, a+1), a
	}
	return qs.walk(ctx, r, start, func(q quadRef) (bool, error) {
		if q.s == last {
			return true, nil
		}
		last = q.s
		if err := cb(nodeID(q.s)); err != nil {
			return false, err
		}
		return true, nil
	})
}

// OptimizeShape replaces a quad filter that pins nodes with one edge range.
func (qs *QuadStore) OptimizeShape(ctx context.Context, s shape.Shape) (shape.Shape, bool, error) {
	// Only a quad filter applies.
	qa, ok := s.(shape.QuadsAction)
	if !ok || len(qa.Filter) == 0 {
		return s, false, nil
	}

	// Gather the filter's IDs, leaving a filter with a foreign ref as it is.
	var f quadRef
	for d, ref := range qa.Filter {
		id, ok := asNodeID(ref)
		if !ok {
			return s, false, nil
		}
		switch d {
		case quad.Subject:
			f.s = id
		case quad.Predicate:
			f.p = id
		case quad.Object:
			f.o = id
		case quad.Label:
			f.l = id
		default:
			return s, false, nil
		}
	}

	// A filter that pins an endpoint becomes that endpoint's range.
	if f.s == 0 && f.o == 0 {
		return s, false, nil
	}
	return qa.SimplifyFrom(rangeShape{qs: qs, r: rangeOf(f)}), true, nil
}

// rangeShape is the query shape of an edge range of the store that built it.
type rangeShape struct {
	qs *QuadStore
	r  edgeRange
}

// BuildIterator returns the range's iterator. The range's IDs belong to the
// store that optimized the shape, which a caching wrapper passed as qs
// reads through, so the iterator reads that store.
func (s rangeShape) BuildIterator(ctx context.Context, qs graph.QuadStore) iterator.Shape {
	return s.qs.newEdgeScan(s.r)
}

// Optimize returns the shape unchanged.
func (s rangeShape) Optimize(ctx context.Context, r shape.Optimizer) (shape.Shape, bool, error) {
	return s, false, nil
}
