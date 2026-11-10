package kvtx_cayley_nodegraph

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// errPastMax stops ChangedQuads at its bound.
var errPastMax = errors.New("node graph: changed quads past the bound")

// NodeKeyPrefix returns the prefix of the node keys, which hold every edge: a
// tree diff limited to it lists every changed quad.
func NodeKeyPrefix() []byte {
	return bytes.Clone(nodePrefix)
}

// WalkChangedQuads calls fn with the quad each out key in keys names, keys
// being keys whose values differ between the trees of before and after, as a
// tree diff lists them. added is whether after holds the quad; otherwise
// before does. Other keys are skipped.
//
// Node IDs are never reused, so a key held by both trees names the same quad
// in each, and the changed quads are exactly the changed out keys. A changed
// meta or ID map key names a node created, deleted or pinned, whose quads
// show as out keys of their own. Edge values are empty, so a changed out key
// is in exactly one tree, and one read of the after tree finds which.
func WalkChangedQuads(ctx context.Context, before, after kvtx.TxOps, keys [][]byte, fn func(q quad.Quad, added bool) error) error {
	// Resolve names in each tree through its own cache.
	var trees [2]*QuadStore
	var names [2]*nameCache
	for i, tx := range [2]kvtx.TxOps{before, after} {
		trees[i] = NewQuadStore(tx)
		names[i] = newNameCache(trees[i])
	}

	// Pass on the quad of each out key from the tree that holds it.
	for _, k := range keys {
		if _, tag, ok := parseNodeKey(k); !ok || tag != tagOut {
			continue
		}
		ref, ok := parseEdgeKey(k)
		if !ok {
			continue
		}
		added, err := trees[1].tx.Exists(ctx, k)
		if err != nil {
			return err
		}
		side := 0
		if added {
			side = 1
		}
		q, err := names[side].quad(ctx, ref)
		if err != nil {
			return err
		}
		if err := fn(q, added); err != nil {
			return err
		}
	}
	return nil
}

// ChangedQuads lists the quads WalkChangedQuads passes on, set or deleted
// alike. complete is false when they exceed max.
func ChangedQuads(ctx context.Context, before, after kvtx.TxOps, keys [][]byte, max int) ([]quad.Quad, bool, error) {
	// Collect the walk, stopping once it passes max.
	var quads []quad.Quad
	err := WalkChangedQuads(ctx, before, after, keys, func(q quad.Quad, _ bool) error {
		if len(quads) >= max {
			return errPastMax
		}
		quads = append(quads, q)
		return nil
	})
	if errors.Is(err, errPastMax) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return quads, true, nil
}
