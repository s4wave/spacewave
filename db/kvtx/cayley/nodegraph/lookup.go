package kvtx_cayley_nodegraph

import (
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/graph/iterator"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/query/shape"
	"github.com/pkg/errors"
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
		id, err := c.qs.lookupID(ctx, valueText(v))
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
		err = qs.walk(ctx, r, nil, false, func(q quadRef) (bool, error) {
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

// IterateEdges calls cb with the endpoint and label of each quad that joins
// origin by pred: its object when origin is its subject (dir is
// quad.Subject), its subject when origin is its object (quad.Object). The
// label is nil when the quad has none. Edges come in endpoint ID order, which
// is the order the endpoint nodes were created, oldest first, or newest first
// when reverse, and an endpoint's labeled edges follow its unlabeled one in
// label ID order, or precede it in reverse. The walk starts just past all
// edges of the endpoint after, or at the first edge when after is nil, and
// stops when cb returns false or an error, which it returns.
func (qs *QuadStore) IterateEdges(
	ctx context.Context,
	origin, pred graph.Ref,
	dir quad.Direction,
	after graph.Ref,
	reverse bool,
	cb func(endpoint, label graph.Ref) (bool, error),
) error {
	// Refs from another store name nothing here.
	o, ok := asNodeID(origin)
	if !ok {
		return nil
	}
	p, ok := asNodeID(pred)
	if !ok {
		return nil
	}

	// Walk the origin's out or in range for pred.
	r := edgeRange{node: o, narrow: []uint64{p}}
	switch dir {
	case quad.Subject:
		r.tag, r.filter = tagOut, quadRef{s: o, p: p}
	case quad.Object:
		r.tag, r.filter = tagIn, quadRef{p: p, o: o}
	default:
		return errors.Errorf("edge origin direction %v is neither subject nor object", dir)
	}

	// Seek to after's unlabeled key and skip after's edges: forward they
	// start there, and in reverse only that one is reached, since its
	// labeled keys sort after it.
	var start []byte
	var skip uint64
	if a, ok := asNodeID(after); ok {
		start, skip = rangeKey(o, r.tag, p, a), a
	}
	return qs.walk(ctx, r, start, reverse, func(q quadRef) (bool, error) {
		// Report the endpoint at the far end from origin with its label.
		endpoint := q.o
		if dir == quad.Object {
			endpoint = q.s
		}
		if endpoint == skip {
			return true, nil
		}
		var label graph.Ref
		if q.l != 0 {
			label = nodeID(q.l)
		}
		return cb(nodeID(endpoint), label)
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
