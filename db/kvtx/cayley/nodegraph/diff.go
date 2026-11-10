package kvtx_cayley_nodegraph

import (
	"context"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/kvtx"
)

// ChangedQuads lists the quads set or deleted between the graphs in before and
// after, from keys, the keys whose values differ between their trees, as a tree
// diff lists them. complete is false when the quads exceed max.
//
// An out key held by one tree names a quad of that tree. An out key held by
// both names the same IDs in each, but an ID freed at the end of the tree and
// allocated again names another value, so equal keys can name different
// quads. Such an ID shows in keys as a meta key whose value differs, and
// ChangedQuads walks that node's edges in both trees. Predicates and labels
// are pinned and never freed, so only a subject or an object is reused, and
// its own ranges hold every edge it names.
func ChangedQuads(ctx context.Context, before, after kvtx.TxOps, keys [][]byte, max int) ([]quad.Quad, bool, error) {
	d := &quadDiff{max: max}
	for i, tx := range [2]kvtx.TxOps{before, after} {
		qs := NewQuadStore(tx, nil)
		d.trees[i] = qs
		d.names[i] = newNameCache(qs)
		d.seen[i] = make(map[quadRef]struct{})
	}
	for _, k := range keys {
		more, err := d.key(ctx, k)
		if err != nil || !more {
			return nil, false, err
		}
	}
	return d.quads, true, nil
}

// quadDiff collects the changed quads of two trees, each once per tree.
type quadDiff struct {
	trees [2]*QuadStore
	names [2]*nameCache
	seen  [2]map[quadRef]struct{}
	quads []quad.Quad
	max   int
}

// key adds the quads one changed key names. It reports false when the quads
// exceed max.
func (d *quadDiff) key(ctx context.Context, k []byte) (bool, error) {
	id, tag, ok := parseNodeKey(k)
	if !ok {
		return true, nil
	}
	switch tag {
	case tagOut:
		// The quad of an out key, in the tree that holds it. Edge values are
		// empty, so a changed out key is in exactly one tree, and one read of
		// the after tree finds which.
		q, ok := parseEdgeKey(k)
		if !ok {
			return true, nil
		}
		found, err := d.trees[1].tx.Exists(ctx, k)
		if err != nil {
			return false, err
		}
		if found {
			return d.add(ctx, 1, q)
		}
		return d.add(ctx, 0, q)
	case tagMeta:
		// Every edge of a reused ID, in both trees.
		reused, err := d.reused(ctx, id)
		if err != nil || !reused {
			return true, err
		}
		for i, qs := range d.trees {
			for _, tag := range [2]byte{tagOut, tagIn} {
				more := true
				err := qs.walk(ctx, edgeRange{node: id, tag: tag}, nil, func(q quadRef) (bool, error) {
					var addErr error
					more, addErr = d.add(ctx, i, q)
					return more, addErr
				})
				if err != nil || !more {
					return more, err
				}
			}
		}
	}
	return true, nil
}

// reused reports whether both trees hold node id with different values.
func (d *quadDiff) reused(ctx context.Context, id uint64) (bool, error) {
	// Read the node in each tree; a node missing from either is not reused.
	before, err := d.trees[0].readMeta(ctx, id)
	if err != nil || before == nil {
		return false, err
	}
	after, err := d.trees[1].readMeta(ctx, id)
	if err != nil || after == nil {
		return false, err
	}
	return !before.GetValue().EqualVT(after.GetValue()), nil
}

// add resolves q in tree i and adds it once. It reports false when the quads
// would exceed max.
func (d *quadDiff) add(ctx context.Context, i int, q quadRef) (bool, error) {
	// Skip a quad already added from this tree, and stop at the bound.
	if _, ok := d.seen[i][q]; ok {
		return true, nil
	}
	if len(d.quads) >= d.max {
		return false, nil
	}

	// Resolve the quad's values in the tree that holds it.
	d.seen[i][q] = struct{}{}
	cq, err := d.names[i].quad(ctx, q)
	if err != nil {
		return false, err
	}
	d.quads = append(d.quads, cq)
	return true, nil
}
