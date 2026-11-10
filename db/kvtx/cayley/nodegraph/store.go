// Package kvtx_cayley_nodegraph is a Cayley quad store that keeps each node's
// quads under the node's own keys.
//
// A quad is two empty keys: an out key under its subject and an in key under
// its object. There is no log, posting list, reference count or counter. A
// node lives while it has an edge, and a predicate or label node is pinned,
// since no range of its own shows whether a quad still names it. Every lookup
// pins a subject or an object; the store has no predicate-first or
// label-first range, and answers those directions with a scan of every out key.
package kvtx_cayley_nodegraph

import (
	"context"
	"sync"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/graph/iterator"
	"github.com/aperturerobotics/cayley/graph/refs"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/quad/pquads"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// ErrInvalidQuad is returned for a quad without a subject, predicate or object.
var ErrInvalidQuad = errors.New("node graph: quad needs a subject, predicate and object")

// QuadStore is a node graph quad store over one transaction.
//
// Concurrent calls are safe only for readers; one writer at a time calls
// ApplyDeltas, as the World serializes its writers.
type QuadStore struct {
	// tx holds the graph keys, including this transaction's held writes.
	tx kvtx.TxOps

	// idMtx guards nextID, storedID and pinned.
	idMtx sync.Mutex
	// nextID is the next node ID to allocate, or 0 before loadNextID.
	nextID uint64
	// storedID is the next ID the tree's key holds, as read or last written.
	storedID uint64
	// pinned holds the nodes this transaction has seen pinned.
	pinned map[uint64]struct{}
}

// NewQuadStore opens the store over tx.
func NewQuadStore(tx kvtx.TxOps) *QuadStore {
	return &QuadStore{tx: tx}
}

// nodeID is a reference to a node.
type nodeID uint64

// Key returns the comparable key of the node.
func (n nodeID) Key() any { return n }

// quadRef is a reference to a quad: its node IDs, with a zero label for none.
type quadRef struct {
	s, p, o, l uint64
}

// Key returns the comparable key of the quad.
func (q quadRef) Key() any { return q }

// dir returns the node ID of the quad in direction d.
func (q quadRef) dir(d quad.Direction) uint64 {
	switch d {
	case quad.Subject:
		return q.s
	case quad.Predicate:
		return q.p
	case quad.Object:
		return q.o
	case quad.Label:
		return q.l
	}
	return 0
}

// asNodeID returns the ID of a node reference.
func asNodeID(ref graph.Ref) (uint64, bool) {
	n, ok := ref.(nodeID)
	return uint64(n), ok && n != 0
}

// lookupID returns the ID of a value's text, or 0 when it has none.
func (qs *QuadStore) lookupID(ctx context.Context, text string) (uint64, error) {
	// Read the ID map entry; a missing one is no node.
	data, found, err := qs.tx.Get(ctx, valueKey(text))
	if err != nil || !found {
		return 0, err
	}

	// Decode the stored ID.
	id, ok := decodeID(data)
	if !ok {
		return 0, errors.Errorf("node graph: invalid id for value %q", text)
	}
	return id, nil
}

// readMeta reads a node's meta, or nil when the node does not exist.
func (qs *QuadStore) readMeta(ctx context.Context, id uint64) (*NodeMeta, error) {
	// Read the meta key; a missing one is no node.
	data, found, err := qs.tx.Get(ctx, metaKey(id))
	if err != nil || !found {
		return nil, err
	}

	// Decode it.
	meta := &NodeMeta{}
	if err := meta.UnmarshalVT(data); err != nil {
		return nil, errors.Wrapf(err, "node graph: node %d meta", id)
	}
	return meta, nil
}

// ValueOf returns the reference of a value, or nil when no quad names it.
func (qs *QuadStore) ValueOf(ctx context.Context, v quad.Value) (graph.Ref, error) {
	if v == nil {
		return nil, nil
	}
	id, err := qs.lookupID(ctx, v.String())
	if err != nil || id == 0 {
		return nil, err
	}
	return nodeID(id), nil
}

// NameOf returns the value of a node reference, or nil when it is not found.
func (qs *QuadStore) NameOf(ctx context.Context, ref graph.Ref) (quad.Value, error) {
	if v, ok := ref.(refs.PreFetchedValue); ok {
		return v.NameOf(), nil
	}
	id, ok := asNodeID(ref)
	if !ok {
		return nil, nil
	}
	return qs.nameOfID(ctx, id)
}

// nameOfID returns the value of a node ID.
func (qs *QuadStore) nameOfID(ctx context.Context, id uint64) (quad.Value, error) {
	meta, err := qs.readMeta(ctx, id)
	if err != nil || meta == nil {
		return nil, err
	}
	return meta.GetValue().ToNative(), nil
}

// Quad returns the quad of a reference.
func (qs *QuadStore) Quad(ctx context.Context, ref graph.Ref) (quad.Quad, error) {
	qr, ok := ref.(quadRef)
	if !ok {
		return quad.Quad{}, nil
	}
	return newNameCache(qs).quad(ctx, qr)
}

// QuadDirection returns the node of a quad in direction d.
func (qs *QuadStore) QuadDirection(ctx context.Context, ref graph.Ref, d quad.Direction) (graph.Ref, error) {
	// A ref of another kind has no directions here.
	qr, ok := ref.(quadRef)
	if !ok {
		return nil, nil
	}

	// An unset direction, such as no label, has no node.
	id := qr.dir(d)
	if id == 0 {
		return nil, nil
	}
	return nodeID(id), nil
}

// QuadIterator returns the quads that have ref in direction d.
func (qs *QuadStore) QuadIterator(ctx context.Context, d quad.Direction, ref graph.Ref) iterator.Shape {
	id, ok := asNodeID(ref)
	if !ok {
		return iterator.NewNull()
	}
	switch d {
	case quad.Subject:
		return qs.newEdgeScan(edgeRange{node: id, tag: tagOut})
	case quad.Object:
		return qs.newEdgeScan(edgeRange{node: id, tag: tagIn})
	case quad.Predicate:
		return qs.newEdgeScan(edgeRange{filter: quadRef{p: id}})
	case quad.Label:
		return qs.newEdgeScan(edgeRange{filter: quadRef{l: id}})
	}
	return iterator.NewNull()
}

// QuadIteratorSize returns the number of quads that have ref in direction d.
//
// A subject or object range is counted up to sizeProbeLimit and reported
// exact below it; unlike the shapes' Stats, which the planner calls while
// choosing a plan, this is a direct question a caller pays for once. The store
// has no predicate-first or label-first range, so those directions report an
// inexact World-sized estimate, which keeps the optimizer on a pinned endpoint.
func (qs *QuadStore) QuadIteratorSize(ctx context.Context, d quad.Direction, ref graph.Ref) (refs.Size, error) {
	id, ok := asNodeID(ref)
	if !ok {
		return refs.Size{Exact: true}, nil
	}
	switch d {
	case quad.Subject:
		return qs.probeSize(ctx, edgeRange{node: id, tag: tagOut})
	case quad.Object:
		return qs.probeSize(ctx, edgeRange{node: id, tag: tagIn})
	}
	return refs.Size{Value: worldSizeEstimate}, nil
}

// Stats returns the number of nodes and quads.
//
// The exact counts walk every node. The estimate reads no storage beyond the
// store's one read of the next node ID: the planner asks for it while
// choosing a plan, and the tree's key count would flush the write buffer. The
// store keeps no quad count, so it estimates one quad per node.
func (qs *QuadStore) Stats(ctx context.Context, exact bool) (graph.Stats, error) {
	// Estimate from the next node ID.
	if !exact {
		n, err := qs.nodeEstimate(ctx)
		if err != nil {
			return graph.Stats{}, err
		}
		return graph.Stats{Nodes: refs.Size{Value: n}, Quads: refs.Size{Value: n}}, nil
	}

	// Count meta and out keys in one walk of the node keys.
	var nodes, quads int64
	it := qs.tx.Iterate(ctx, nodePrefix, true, false)
	defer it.Close()
	for it.Next() {
		_, tag, ok := parseNodeKey(it.Key())
		switch {
		case !ok:
		case tag == tagMeta:
			nodes++
		case tag == tagOut:
			quads++
		}
	}
	if err := it.Err(); err != nil {
		return graph.Stats{}, err
	}
	return graph.Stats{
		Nodes: refs.Size{Value: nodes, Exact: true},
		Quads: refs.Size{Value: quads, Exact: true},
	}, nil
}

// NewQuadWriter returns a writer that adds quads.
func (qs *QuadStore) NewQuadWriter(ctx context.Context) (quad.WriteCloser, error) {
	return &quadWriter{qs: qs}, nil
}

// NodesAllIterator returns every node.
func (qs *QuadStore) NodesAllIterator(ctx context.Context) iterator.Shape {
	return &nodeScan{qs: qs}
}

// QuadsAllIterator returns every quad.
func (qs *QuadStore) QuadsAllIterator(ctx context.Context) iterator.Shape {
	return qs.newEdgeScan(edgeRange{})
}

// Close releases nothing: the transaction belongs to the caller.
func (qs *QuadStore) Close() error { return nil }

// quadWriter adds quads through ApplyDeltas.
type quadWriter struct {
	qs *QuadStore
}

// WriteQuad adds one quad.
func (w *quadWriter) WriteQuad(ctx context.Context, q quad.Quad) error {
	_, err := w.WriteQuads(ctx, []quad.Quad{q})
	return err
}

// WriteQuads adds quads, ignoring those that exist.
func (w *quadWriter) WriteQuads(ctx context.Context, buf []quad.Quad) (int, error) {
	deltas := make([]graph.Delta, len(buf))
	for i, q := range buf {
		deltas[i] = graph.Delta{Quad: q, Action: graph.Add}
	}
	if err := w.qs.ApplyDeltas(ctx, deltas, graph.IgnoreOpts{IgnoreDup: true}); err != nil {
		return 0, err
	}
	return len(buf), nil
}

// Close does nothing.
func (w *quadWriter) Close() error { return nil }

// newNodeMeta builds the meta of a new node.
func newNodeMeta(v quad.Value, pinned bool) *NodeMeta {
	return &NodeMeta{Value: pquads.MakeValue(v), Pinned: pinned}
}

// _ is a type assertion
var _ graph.QuadStore = (*QuadStore)(nil)
