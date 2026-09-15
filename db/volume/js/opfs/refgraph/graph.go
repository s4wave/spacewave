// Package refgraph stores the garbage-collection ownership graph in the OPFS
// volume catalogue.
package refgraph

import (
	"context"
	"encoding/binary"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
)

// graphPrefix separates graph records from the other volume catalogue records.
const graphPrefix byte = 0x02

// Graph stores graph records in one transactional key/value namespace.
type Graph struct {
	// store owns the graph namespace within the volume catalogue.
	store kvtx.Store
}

// NewGraph constructs a graph over store.
func NewGraph(store kvtx.Store) *Graph {
	return &Graph{store: kvtx_prefixer.NewPrefixer(store, []byte{graphPrefix})}
}

// graphKey encodes a record kind followed by unambiguous full node IRIs.
func graphKey(kind byte, nodes ...string) []byte {
	size := 1
	for _, node := range nodes {
		size += 4 + len(node)
	}

	key := make([]byte, 1, size)
	key[0] = kind
	for _, node := range nodes {
		key = binary.BigEndian.AppendUint32(key, uint32(len(node))) //nolint:gosec // graph node keys are bounded by the OPFS record size contract.
		key = append(key, node...)
	}
	return key
}

// transaction retries complete graph operations against a fresh consistent view.
func (g *Graph) transaction(ctx context.Context, write bool, body func(context.Context, kvtx.Tx) error) error {
	return kvtx.RunTransaction(ctx, write, func(ctx context.Context) (kvtx.Tx, error) {
		return g.store.NewTransaction(ctx, write)
	}, body)
}

// AddRef atomically inserts both edge directions and endpoint inventory records.
func (g *Graph) AddRef(ctx context.Context, subject, object string) error {
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		return g.addRefTx(ctx, tx, subject, object)
	})
}

// RemoveRef removes an exact edge without creating an orphan mark.
func (g *Graph) RemoveRef(ctx context.Context, subject, object string) error {
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		_, err := g.removeRefTx(ctx, tx, subject, object)
		return err
	})
}

// AddRoot records a permanent inventory entry and root membership atomically.
func (g *Graph) AddRoot(ctx context.Context, node string) error {
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		for _, kind := range []byte{'n', 'r'} {
			key := graphKey(kind, node)
			found, err := tx.Exists(ctx, key)
			if err != nil {
				return err
			}
			if found {
				continue
			}
			if err := tx.Set(ctx, key, []byte(node)); err != nil {
				return err
			}
		}
		return nil
	})
}

// RemoveRoot removes root membership without changing the node's edges.
func (g *Graph) RemoveRoot(ctx context.Context, node string) error {
	return g.deleteRecord(ctx, graphKey('r', node))
}

// RemoveNode removes inventory after the collector has handled its edges.
func (g *Graph) RemoveNode(ctx context.Context, node string) error {
	return g.deleteRecord(ctx, graphKey('n', node))
}

// deleteRecord treats missing graph metadata as a no-op.
func (g *Graph) deleteRecord(ctx context.Context, key []byte) error {
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		found, err := tx.Exists(ctx, key)
		if err != nil || !found {
			return err
		}
		return tx.Delete(ctx, key)
	})
}

// list returns the complete answer requested by the graph traversal interface.
// Only callback-owned answer state grows with the result cardinality.
func (g *Graph) list(ctx context.Context, prefix []byte) ([]string, error) {
	var answer []string
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		answer = nil
		return tx.ScanPrefix(ctx, prefix, func(_, value []byte) error {
			answer = append(answer, string(value))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return answer, nil
}

// GetOutgoingRefs returns all current targets of one complete source identity.
func (g *Graph) GetOutgoingRefs(ctx context.Context, node string) ([]string, error) {
	return g.list(ctx, graphKey('f', node))
}

// GetIncomingRefs returns all current owners of one complete target identity.
func (g *Graph) GetIncomingRefs(ctx context.Context, node string) ([]string, error) {
	return g.list(ctx, graphKey('i', node))
}

// GetUnreferencedNodes returns nodes tracked for collector reconsideration.
func (g *Graph) GetUnreferencedNodes(ctx context.Context) ([]string, error) {
	return g.GetOutgoingRefs(ctx, block_gc.NodeUnreferenced)
}

// IterateNodes supplies the explicit full node inventory requested by marking.
func (g *Graph) IterateNodes(ctx context.Context) ([]string, error) {
	return g.list(ctx, graphKey('n'))
}

// GetRootNodes supplies the collector's registered roots.
func (g *Graph) GetRootNodes(ctx context.Context) ([]string, error) {
	return g.list(ctx, graphKey('r'))
}

// HasIncomingRefs ignores the bookkeeping edge from the unreferenced root.
func (g *Graph) HasIncomingRefs(ctx context.Context, node string) (bool, error) {
	return g.HasIncomingRefsExcluding(ctx, node)
}

// HasIncomingRefsExcluding stops at the first owner outside the caller's exclusions.
func (g *Graph) HasIncomingRefsExcluding(ctx context.Context, node string, excluded ...string) (bool, error) {
	ignored := make(map[string]struct{}, len(excluded)+1)
	ignored[block_gc.NodeUnreferenced] = struct{}{}
	for _, source := range excluded {
		ignored[source] = struct{}{}
	}
	found := false
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		found = false
		it := tx.Iterate(ctx, graphKey('i', node), true, false)
		defer it.Close()
		for it.Next() {
			value, err := it.Value()
			if err != nil {
				return err
			}
			if _, skip := ignored[string(value)]; !skip {
				found = true
				break
			}
		}
		return it.Err()
	})
	return found, err
}

// RemoveNodeRefs removes the observed outgoing targets in bounded transactions.
// New unrelated edges may be added after this operation's initial snapshot.
func (g *Graph) RemoveNodeRefs(ctx context.Context, node string, markOrphaned bool) ([]string, error) {
	targets, err := g.GetOutgoingRefs(ctx, node)
	if err != nil {
		return nil, err
	}
	for offset := 0; offset < len(targets); offset += maxBatchEdges {
		end := min(offset+maxBatchEdges, len(targets))
		err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
			if markOrphaned {
				removes := make([]block_gc.RefEdge, 0, end-offset)
				for _, target := range targets[offset:end] {
					removes = append(removes, block_gc.RefEdge{Subject: node, Object: target})
				}
				return g.applyRefBatchTx(ctx, tx, nil, removes)
			}
			for _, target := range targets[offset:end] {
				if _, err := g.removeRefTx(ctx, tx, node, target); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return targets, nil
}

// AddBlockRef preserves the collector's canonical block identity vocabulary.
func (g *Graph) AddBlockRef(ctx context.Context, source, target *block.BlockRef) error {
	from, to := block_gc.BlockIRI(source), block_gc.BlockIRI(target)
	if from == "" || to == "" {
		return nil
	}
	return g.AddRef(ctx, from, to)
}

// AddObjectRoot links an object identity to its current content root.
func (g *Graph) AddObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	target := block_gc.BlockIRI(ref)
	if target == "" {
		return nil
	}
	return g.AddRef(ctx, block_gc.ObjectIRI(objectKey), target)
}

// RemoveObjectRoot removes an exact object-to-block ownership edge.
func (g *Graph) RemoveObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	target := block_gc.BlockIRI(ref)
	if target == "" {
		return nil
	}
	return g.RemoveRef(ctx, block_gc.ObjectIRI(objectKey), target)
}

// Close leaves the shared volume engine's lifetime with its volume owner.
func (g *Graph) Close() error { return nil }

// _ verifies all graph operations required by the collector.
var _ block_gc.CollectorGraph = (*Graph)(nil)
