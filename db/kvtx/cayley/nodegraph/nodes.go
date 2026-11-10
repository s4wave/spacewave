package kvtx_cayley_nodegraph

import (
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/graph/iterator"
	"github.com/aperturerobotics/cayley/graph/refs"
	"github.com/s4wave/spacewave/db/kvtx"
)

// nodeScan is the shape of every node.
type nodeScan struct {
	qs *QuadStore
}

// String describes the scan.
func (s *nodeScan) String() string { return "NodeGraphNodes" }

// Iterate returns a scanner over every node.
func (s *nodeScan) Iterate(ctx context.Context) iterator.Scanner {
	return &nodeScanner{qs: s.qs}
}

// Lookup returns an index over every node.
func (s *nodeScan) Lookup(ctx context.Context) iterator.Index {
	return &nodeIndex{qs: s.qs}
}

// Stats estimates the number of nodes from the next node ID, which the store
// holds once it has read the last ID.
func (s *nodeScan) Stats(ctx context.Context) (iterator.Costs, error) {
	n, err := s.qs.nodeEstimate(ctx)
	return iterator.Costs{ContainsCost: 1, NextCost: 1, Size: refs.Size{Value: n}}, err
}

// Optimize returns the scan unchanged.
func (s *nodeScan) Optimize(ctx context.Context) (iterator.Shape, bool, error) {
	return s, false, nil
}

// SubIterators returns nil.
func (s *nodeScan) SubIterators() []iterator.Shape { return nil }

// nodeScanner walks the meta keys, seeking past each node's edges.
type nodeScanner struct {
	qs  *QuadStore
	it  kvtx.Iterator
	cur graph.Ref
	err error
}

// Next advances to the next node.
func (it *nodeScanner) Next(ctx context.Context) bool {
	var seek []byte
	if it.it == nil {
		if it.err != nil {
			return false
		}
		it.it = it.qs.tx.Iterate(ctx, nodePrefix, true, false)
		seek = nodePrefix
	}
	for {
		if seek != nil {
			if it.err = it.it.Seek(seek); it.err != nil {
				return false
			}
			seek = nil
		} else {
			it.it.Next()
		}
		if !it.it.Valid() {
			it.err, it.cur = it.it.Err(), nil
			return false
		}
		id, tag, ok := parseNodeKey(it.it.Key())
		if !ok {
			continue
		}
		if tag != tagMeta {
			seek = nodeKey(make([]byte, 0, nodeKeyLen), id+1)
			continue
		}
		it.cur = nodeID(id)
		return true
	}
}

// Result returns the current node.
func (it *nodeScanner) Result(ctx context.Context) (graph.Ref, error) { return it.cur, it.err }

// TagResults adds no tags.
func (it *nodeScanner) TagResults(ctx context.Context, dst map[string]graph.Ref) error { return nil }

// NextPath returns false.
func (it *nodeScanner) NextPath(ctx context.Context) bool { return false }

// Err returns the walk error.
func (it *nodeScanner) Err() error { return it.err }

// Close closes the iterator.
func (it *nodeScanner) Close() error {
	if it.it != nil {
		it.it.Close()
	}
	return nil
}

// String describes the scanner.
func (it *nodeScanner) String() string { return "NodeGraphNodes" }

// nodeIndex checks whether a node exists.
type nodeIndex struct {
	qs  *QuadStore
	cur graph.Ref
	err error
}

// Contains reports whether ref is a node with a meta key.
func (it *nodeIndex) Contains(ctx context.Context, ref graph.Ref) (bool, error) {
	// A ref of another kind is not a node here.
	id, ok := asNodeID(ref)
	if !ok {
		return false, nil
	}

	// A node exists while its meta key does.
	found, err := it.qs.tx.Exists(ctx, metaKey(id))
	if err != nil || !found {
		it.err = err
		return false, err
	}
	it.cur = ref
	return true, nil
}

// Result returns the last contained node.
func (it *nodeIndex) Result(ctx context.Context) (graph.Ref, error) { return it.cur, it.err }

// TagResults adds no tags.
func (it *nodeIndex) TagResults(ctx context.Context, dst map[string]graph.Ref) error { return nil }

// NextPath returns false.
func (it *nodeIndex) NextPath(ctx context.Context) bool { return false }

// Err returns the last error.
func (it *nodeIndex) Err() error { return it.err }

// Close does nothing.
func (it *nodeIndex) Close() error { return nil }

// String describes the index.
func (it *nodeIndex) String() string { return "NodeGraphNodes" }
