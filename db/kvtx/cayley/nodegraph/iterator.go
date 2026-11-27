package kvtx_cayley_nodegraph

import (
	"context"
	"fmt"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/graph/iterator"
	"github.com/aperturerobotics/cayley/graph/refs"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/kvtx"
)

const (
	// sizeProbeLimit is the most keys a size probe counts.
	sizeProbeLimit = 32
	// worldSizeEstimate is the size reported for a scan of every quad, which
	// the store cannot count without walking it.
	worldSizeEstimate = 1 << 40
	// edgeRangeEstimate is the size a planner sees for one node's edge range.
	// The store keeps no counts and Stats reads no storage, so every anchored
	// range reports the same inexact fanout.
	edgeRangeEstimate = 20
)

// edgeRange is a set of quads: a range of one node's edges, or every out key
// when node is zero, filtered to the quads that match filter.
type edgeRange struct {
	// node is the node whose edges the range walks, or zero for every node.
	node uint64
	// tag is tagOut or tagIn when node is set.
	tag byte
	// narrow are the IDs after the tag that every key in the range starts
	// with: the predicate, then the other endpoint, then the label.
	narrow []uint64
	// filter holds the IDs a quad must have. A zero ID matches any.
	filter quadRef
}

// rangeOf returns the range of the quads that match f, walking the node a
// direction pins and narrowing it by the IDs that follow in its keys.
func rangeOf(f quadRef) edgeRange {
	switch {
	case f.s != 0:
		return edgeRange{node: f.s, tag: tagOut, narrow: narrowOf(f.p, f.o, f.l), filter: f}
	case f.o != 0:
		return edgeRange{node: f.o, tag: tagIn, narrow: narrowOf(f.p, f.s, f.l), filter: f}
	}
	return edgeRange{filter: f}
}

// narrowOf returns the leading IDs of an edge key's suffix that are set.
func narrowOf(pred, other, label uint64) []uint64 {
	switch {
	case pred == 0:
		return nil
	case other == 0:
		return []uint64{pred}
	case label == 0:
		return []uint64{pred, other}
	}
	return []uint64{pred, other, label}
}

// matches reports whether q matches the filter.
func (r edgeRange) matches(q quadRef) bool {
	f := r.filter
	return (f.s == 0 || f.s == q.s) &&
		(f.p == 0 || f.p == q.p) &&
		(f.o == 0 || f.o == q.o) &&
		(f.l == 0 || f.l == q.l)
}

// prefix returns the key prefix the range walks.
func (r edgeRange) prefix() []byte {
	if r.node == 0 {
		return nodePrefix
	}
	return rangeKey(r.node, r.tag, r.narrow...)
}

// edgeCursor walks the quads of an edge range in key order, or in reverse key
// order, over one open iterator.
type edgeCursor struct {
	r  edgeRange
	it kvtx.Iterator
	// reverse is set when the cursor walks from the range's last key down.
	reverse bool
	// seeking is set when the next step seeks to seek instead of stepping.
	// A nil seek is the end of a reverse range.
	seeking bool
	seek    []byte
	err     error
}

// openCursor opens a cursor on r that starts at the first key at or past
// start in the walk's direction, or at the range's first key in that
// direction when start is nil.
func (qs *QuadStore) openCursor(ctx context.Context, r edgeRange, start []byte, reverse bool) *edgeCursor {
	prefix := r.prefix()
	if start == nil && !reverse {
		start = prefix
	}
	return &edgeCursor{
		r:       r,
		it:      qs.tx.Iterate(ctx, prefix, true, reverse),
		reverse: reverse,
		seeking: true,
		seek:    start,
	}
}

// next advances to the next quad that matches the range's filter.
func (c *edgeCursor) next() (quadRef, bool) {
	for c.err == nil {
		if c.seeking {
			c.err, c.seeking, c.seek = c.it.Seek(c.seek), false, nil
			if c.err != nil {
				break
			}
		} else {
			c.it.Next()
		}
		if !c.it.Valid() {
			c.err = c.it.Err()
			break
		}
		k := c.it.Key()
		if c.r.node == 0 {
			// Walk only out keys: step past a meta key, and seek past in
			// keys to the next node forward, or to this node's out keys in
			// reverse, which sort just below its in keys.
			id, tag, ok := parseNodeKey(k)
			if !ok || tag == tagMeta {
				continue
			}
			if tag != tagOut {
				c.seeking, c.seek = true, c.pastInKeys(id)
				continue
			}
		}
		if q, ok := parseEdgeKey(k); ok && c.r.matches(q) {
			return q, true
		}
	}
	return quadRef{}, false
}

// pastInKeys returns the seek key that skips the in keys of node id in the
// cursor's direction: the next node's first key forward, and in reverse the
// bare in tag, which sorts above every out key and below every in key.
func (c *edgeCursor) pastInKeys(id uint64) []byte {
	if c.reverse {
		return append(nodeKey(make([]byte, 0, nodeKeyLen+1), id), tagIn)
	}
	return nodeKey(make([]byte, 0, nodeKeyLen), id+1)
}

// close closes the iterator.
func (c *edgeCursor) close() { c.it.Close() }

// walk calls cb with each quad in the range, in key order or in reverse,
// until cb returns false.
func (qs *QuadStore) walk(
	ctx context.Context,
	r edgeRange,
	start []byte,
	reverse bool,
	cb func(q quadRef) (bool, error),
) error {
	c := qs.openCursor(ctx, r, start, reverse)
	defer c.close()
	for {
		q, ok := c.next()
		if !ok {
			return c.err
		}
		more, err := cb(q)
		if err != nil || !more {
			return err
		}
	}
}

// probeSize counts the quads in the range up to sizeProbeLimit.
func (qs *QuadStore) probeSize(ctx context.Context, r edgeRange) (refs.Size, error) {
	if r.node == 0 {
		return refs.Size{Value: worldSizeEstimate}, nil
	}
	var n int64
	err := qs.walk(ctx, r, nil, false, func(quadRef) (bool, error) {
		n++
		return n < sizeProbeLimit, nil
	})
	return refs.Size{Value: n, Exact: n < sizeProbeLimit}, err
}

// edgeScan is the iterator shape of an edge range.
type edgeScan struct {
	qs *QuadStore
	r  edgeRange
}

// newEdgeScan returns the shape of r.
func (qs *QuadStore) newEdgeScan(r edgeRange) *edgeScan {
	return &edgeScan{qs: qs, r: r}
}

// String describes the scan.
func (s *edgeScan) String() string {
	return fmt.Sprintf("NodeGraphEdges(node=%d tag=%d narrow=%v)", s.r.node, s.r.tag, s.r.narrow)
}

// Iterate returns a scanner over the range.
func (s *edgeScan) Iterate(ctx context.Context) iterator.Scanner {
	return &edgeScanner{s: s}
}

// Lookup returns an index over the range.
func (s *edgeScan) Lookup(ctx context.Context) iterator.Index {
	return &edgeIndex{s: s}
}

// Stats returns the estimated size of the range without reading storage: a
// node's fanout when the range is anchored, and the World's size otherwise.
func (s *edgeScan) Stats(ctx context.Context) (iterator.Costs, error) {
	size := int64(edgeRangeEstimate)
	if s.r.node == 0 {
		size = worldSizeEstimate
	}
	return iterator.Costs{ContainsCost: 1, NextCost: 1, Size: refs.Size{Value: size}}, nil
}

// Optimize returns the scan unchanged.
func (s *edgeScan) Optimize(ctx context.Context) (iterator.Shape, bool, error) {
	return s, false, nil
}

// SubIterators returns nil.
func (s *edgeScan) SubIterators() []iterator.Shape { return nil }

// edgeScanner walks an edge range.
type edgeScanner struct {
	s   *edgeScan
	c   *edgeCursor
	cur graph.Ref
	err error
}

// Next advances to the next quad in the range.
func (it *edgeScanner) Next(ctx context.Context) bool {
	// Open the cursor on the first call.
	if it.c == nil {
		if it.err != nil {
			return false
		}
		it.c = it.s.qs.openCursor(ctx, it.s.r, nil, false)
	}

	// Step it, keeping its error at the end.
	q, ok := it.c.next()
	if !ok {
		it.err, it.cur = it.c.err, nil
		return false
	}
	it.cur = q
	return true
}

// Result returns the current quad.
func (it *edgeScanner) Result(ctx context.Context) (graph.Ref, error) {
	return it.cur, it.err
}

// TagResults adds no tags.
func (it *edgeScanner) TagResults(ctx context.Context, dst map[string]graph.Ref) error { return nil }

// NextPath returns false: each quad has one path.
func (it *edgeScanner) NextPath(ctx context.Context) bool { return false }

// Err returns the walk error.
func (it *edgeScanner) Err() error { return it.err }

// Close closes the cursor.
func (it *edgeScanner) Close() error {
	if it.c != nil {
		it.c.close()
	}
	return nil
}

// String describes the scanner.
func (it *edgeScanner) String() string { return it.s.String() }

// edgeIndex checks whether a quad is in an edge range.
type edgeIndex struct {
	s   *edgeScan
	cur graph.Ref
}

// Contains reports whether ref is a quad in the range. A quad ref comes from
// this store, so it exists. The check is its IDs.
func (it *edgeIndex) Contains(ctx context.Context, ref graph.Ref) (bool, error) {
	// A ref of another kind is not in the range.
	q, ok := ref.(quadRef)
	if !ok {
		return false, nil
	}

	// The quad must sit under the range's node and match its filter.
	r := it.s.r
	if r.node != 0 {
		anchor := quad.Subject
		if r.tag == tagIn {
			anchor = quad.Object
		}
		if q.dir(anchor) != r.node {
			return false, nil
		}
	}
	if !r.matches(q) {
		return false, nil
	}
	it.cur = q
	return true, nil
}

// Result returns the last contained quad.
func (it *edgeIndex) Result(ctx context.Context) (graph.Ref, error) { return it.cur, nil }

// TagResults adds no tags.
func (it *edgeIndex) TagResults(ctx context.Context, dst map[string]graph.Ref) error { return nil }

// NextPath returns false: each quad has one path.
func (it *edgeIndex) NextPath(ctx context.Context) bool { return false }

// Err returns nil.
func (it *edgeIndex) Err() error { return nil }

// Close does nothing.
func (it *edgeIndex) Close() error { return nil }

// String describes the index.
func (it *edgeIndex) String() string { return it.s.String() }
