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
// Node IDs are never reused, so a key held by both trees names the same quad
// in each, and the changed quads are exactly the changed out keys. A changed
// meta or ID map key names a node created, deleted or pinned, whose quads
// show as out keys of their own.
func ChangedQuads(ctx context.Context, before, after kvtx.TxOps, keys [][]byte, max int) ([]quad.Quad, bool, error) {
	d := &quadDiff{max: max}
	for i, tx := range [2]kvtx.TxOps{before, after} {
		qs := NewQuadStore(tx)
		d.trees[i] = qs
		d.names[i] = newNameCache(qs)
	}
	for _, k := range keys {
		more, err := d.key(ctx, k)
		if err != nil || !more {
			return nil, false, err
		}
	}
	return d.quads, true, nil
}

// quadDiff collects the changed quads of two trees.
type quadDiff struct {
	trees [2]*QuadStore
	names [2]*nameCache
	quads []quad.Quad
	max   int
}

// key adds the quad a changed out key names, in the tree that holds it. Edge
// values are empty, so a changed out key is in exactly one tree, and one read
// of the after tree finds which. It reports false when the quads exceed max.
func (d *quadDiff) key(ctx context.Context, k []byte) (bool, error) {
	// Only an out key names a quad, once.
	if _, tag, ok := parseNodeKey(k); !ok || tag != tagOut {
		return true, nil
	}
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
}

// add resolves q in tree i and adds it. It reports false when the quads
// would exceed max.
func (d *quadDiff) add(ctx context.Context, i int, q quadRef) (bool, error) {
	// Stop at the bound.
	if len(d.quads) >= d.max {
		return false, nil
	}

	// Resolve the quad's values in the tree that holds it.
	cq, err := d.names[i].quad(ctx, q)
	if err != nil {
		return false, err
	}
	d.quads = append(d.quads, cq)
	return true, nil
}
