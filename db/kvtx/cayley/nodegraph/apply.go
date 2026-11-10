package kvtx_cayley_nodegraph

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// quadText identifies a quad by its values' text, with "" for no label.
type quadText [4]string

// textOf returns the text of a quad's values.
func textOf(q quad.Quad) quadText {
	var t quadText
	for i, d := range quad.Directions {
		if v := q.Get(d); v != nil {
			t[i] = v.String()
		}
	}
	return t
}

// ApplyDeltas adds and removes quads.
//
// Every delta is checked before any key is written, so a rejected call
// changes nothing. A node left without edges is deleted with its ID map entry
// in the same call, unless it is pinned as a predicate or label.
func (qs *QuadStore) ApplyDeltas(ctx context.Context, in []graph.Delta, opts graph.IgnoreOpts) error {
	// An empty call writes nothing; otherwise one batch resolves each value once.
	if len(in) == 0 {
		return nil
	}
	b := &deltaBatch{qs: qs, ids: make(map[string]uint64), present: make(map[quadText]bool)}

	// Check every delta against the store and the deltas before it.
	apply := make([]bool, len(in))
	for i, d := range in {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !d.Quad.IsValid() {
			return &graph.DeltaError{Delta: d, Err: ErrInvalidQuad}
		}
		exists, err := b.exists(ctx, d.Quad)
		if err != nil {
			return err
		}
		switch d.Action {
		case graph.Add:
			if exists {
				if opts.IgnoreDup {
					continue
				}
				return &graph.DeltaError{Delta: d, Err: graph.ErrQuadExists}
			}
		case graph.Delete:
			if !exists {
				if opts.IgnoreMissing {
					continue
				}
				return &graph.DeltaError{Delta: d, Err: graph.ErrQuadNotExist}
			}
		default:
			return &graph.DeltaError{Delta: d, Err: graph.ErrInvalidAction}
		}
		b.present[textOf(d.Quad)] = d.Action == graph.Add
		apply[i] = true
	}

	// Write the accepted deltas, then delete the nodes they left empty.
	var emptied []uint64
	for i, d := range in {
		if !apply[i] {
			continue
		}
		var err error
		if d.Action == graph.Add {
			err = b.add(ctx, d.Quad)
		} else {
			emptied, err = b.remove(ctx, d.Quad, emptied)
		}
		if err != nil {
			return err
		}
	}
	for _, id := range emptied {
		if err := qs.deleteIfEmpty(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// deltaBatch resolves each distinct value once within one ApplyDeltas call.
type deltaBatch struct {
	qs *QuadStore
	// ids maps value text to node ID, with 0 for a value that has no node.
	ids map[string]uint64
	// present is whether each quad checked in this call exists after the
	// deltas before it.
	present map[quadText]bool
}

// lookup returns the node ID of a value, or 0 when it has no node.
func (b *deltaBatch) lookup(ctx context.Context, v quad.Value) (uint64, error) {
	// A missing value has no node, and a value seen in this call is known.
	if v == nil {
		return 0, nil
	}
	text := v.String()
	if id, ok := b.ids[text]; ok {
		return id, nil
	}

	// Read the ID map once and remember the answer, a miss included.
	id, err := b.qs.lookupID(ctx, text)
	if err != nil {
		return 0, err
	}
	b.ids[text] = id
	return id, nil
}

// resolve returns the quad's node IDs, with ok false when a value has no node.
func (b *deltaBatch) resolve(ctx context.Context, q quad.Quad) (quadRef, bool, error) {
	var ids [4]uint64
	for i, d := range quad.Directions {
		v := q.Get(d)
		if v == nil {
			continue
		}
		id, err := b.lookup(ctx, v)
		if err != nil || id == 0 {
			return quadRef{}, false, err
		}
		ids[i] = id
	}
	return quadRef{s: ids[0], p: ids[1], o: ids[2], l: ids[3]}, true, nil
}

// exists reports whether q exists after the deltas checked before it.
func (b *deltaBatch) exists(ctx context.Context, q quad.Quad) (bool, error) {
	// A quad an earlier delta of this call checked exists as that delta left it.
	if present, ok := b.present[textOf(q)]; ok {
		return present, nil
	}

	// Otherwise the quad exists when its values have nodes and its out key is set.
	qr, ok, err := b.resolve(ctx, q)
	if err != nil || !ok {
		return false, err
	}
	out, _ := edgeKeys(qr)
	return b.qs.tx.Exists(ctx, out)
}

// node returns the ID of a value, creating its node when it has none, and
// pins it when pin is set.
func (b *deltaBatch) node(ctx context.Context, v quad.Value, pin bool) (uint64, error) {
	// Find the value's node, if it has one.
	if v == nil {
		return 0, nil
	}
	id, err := b.lookup(ctx, v)
	if err != nil {
		return 0, err
	}

	// Create a missing node, or pin an existing one.
	if id == 0 {
		id, err = b.qs.createNode(ctx, v, pin)
		if err != nil {
			return 0, err
		}
		b.ids[v.String()] = id
		return id, nil
	}
	if pin {
		err = b.qs.pin(ctx, id)
	}
	return id, err
}

// add writes the out and in keys of a quad that does not exist.
func (b *deltaBatch) add(ctx context.Context, q quad.Quad) error {
	// Resolve the endpoints to their nodes, creating any that are new.
	var qr quadRef
	var err error
	if qr.s, err = b.node(ctx, q.Subject, false); err != nil {
		return err
	}
	if qr.o, err = b.node(ctx, q.Object, false); err != nil {
		return err
	}

	// Resolve the predicate and label the same way, pinning them.
	if qr.p, err = b.node(ctx, q.Predicate, true); err != nil {
		return err
	}
	if qr.l, err = b.node(ctx, q.Label, true); err != nil {
		return err
	}

	// Write the edge under both endpoints.
	out, in := edgeKeys(qr)
	if err := b.qs.tx.Set(ctx, out, []byte{}); err != nil {
		return err
	}
	return b.qs.tx.Set(ctx, in, []byte{})
}

// remove deletes the out and in keys of a quad that exists, and appends its
// endpoints to emptied for the liveness check.
func (b *deltaBatch) remove(ctx context.Context, q quad.Quad, emptied []uint64) ([]uint64, error) {
	// Resolve the quad, which the check found, to its node IDs.
	qr, ok, err := b.resolve(ctx, q)
	if err != nil {
		return emptied, err
	}
	if !ok {
		return emptied, errors.Errorf("node graph: removed quad %v has no node", q)
	}

	// Delete the edge under both endpoints.
	out, in := edgeKeys(qr)
	if err := b.qs.tx.Delete(ctx, out); err != nil {
		return emptied, err
	}
	if err := b.qs.tx.Delete(ctx, in); err != nil {
		return emptied, err
	}

	// Queue each endpoint once for the liveness check.
	for _, id := range [2]uint64{qr.s, qr.o} {
		if !containsID(emptied, id) {
			emptied = append(emptied, id)
		}
	}
	return emptied, nil
}

// containsID reports whether ids holds id.
func containsID(ids []uint64, id uint64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// createNode allocates a node for v and writes its ID map entry and meta.
func (qs *QuadStore) createNode(ctx context.Context, v quad.Value, pinned bool) (uint64, error) {
	// Allocate the ID and encode the meta before writing either key.
	id, err := qs.allocID(ctx)
	if err != nil {
		return 0, err
	}
	meta, err := newNodeMeta(v, pinned).MarshalVT()
	if err != nil {
		return 0, err
	}

	// Write the ID map entry and the meta, and remember a pin.
	if err := qs.tx.Set(ctx, valueKey(v.String()), encodeID(id)); err != nil {
		return 0, err
	}
	if err := qs.tx.Set(ctx, metaKey(id), meta); err != nil {
		return 0, err
	}
	if pinned {
		qs.markPinned(id)
	}
	return id, nil
}

// pin marks an existing node as a predicate or label, once per transaction.
func (qs *QuadStore) pin(ctx context.Context, id uint64) error {
	// Read the meta of a node this transaction has not seen pinned.
	if qs.isPinned(id) {
		return nil
	}
	meta, err := qs.readMeta(ctx, id)
	if err != nil {
		return err
	}
	if meta == nil {
		return errors.Errorf("node graph: node %d has no meta", id)
	}

	// Set the pin in the meta once, and remember it.
	if !meta.GetPinned() {
		meta.Pinned = true
		data, err := meta.MarshalVT()
		if err != nil {
			return err
		}
		if err := qs.tx.Set(ctx, metaKey(id), data); err != nil {
			return err
		}
	}
	qs.markPinned(id)
	return nil
}

// isPinned reports whether this transaction has seen id pinned.
func (qs *QuadStore) isPinned(id uint64) bool {
	qs.idMtx.Lock()
	defer qs.idMtx.Unlock()
	_, ok := qs.pinned[id]
	return ok
}

// markPinned records that id is pinned.
func (qs *QuadStore) markPinned(id uint64) {
	qs.idMtx.Lock()
	defer qs.idMtx.Unlock()
	if qs.pinned == nil {
		qs.pinned = make(map[uint64]struct{})
	}
	qs.pinned[id] = struct{}{}
}

// allocID returns a new node ID, one past the last.
func (qs *QuadStore) allocID(ctx context.Context) (uint64, error) {
	// Load the next ID under the lock on first use.
	qs.idMtx.Lock()
	defer qs.idMtx.Unlock()
	if err := qs.loadNextID(ctx); err != nil {
		return 0, err
	}

	// Take it and advance.
	id := qs.nextID
	qs.nextID++
	return id, nil
}

// nodeEstimate returns the number of node IDs allocated, an upper bound on the
// live nodes: a node deleted below the last ID leaves a gap.
func (qs *QuadStore) nodeEstimate(ctx context.Context) (int64, error) {
	qs.idMtx.Lock()
	defer qs.idMtx.Unlock()
	if err := qs.loadNextID(ctx); err != nil {
		return 0, err
	}
	return int64(qs.nextID - 1), nil //nolint:gosec // IDs stay far below 1<<63.
}

// loadNextID sets nextID on first use from the last ID in the tree under the
// write buffer, which holds no ID this store did not allocate: one reverse
// seek per store. The caller holds idMtx.
func (qs *QuadStore) loadNextID(ctx context.Context) error {
	// Loaded once per store.
	if qs.nextID != 0 {
		return nil
	}

	// One past the largest committed ID.
	last, err := lastNodeID(ctx, qs.committed)
	if err != nil {
		return err
	}
	qs.nextID = last + 1
	return nil
}

// lastNodeID returns the largest node ID in tx, or 0 when it has none.
func lastNodeID(ctx context.Context, tx kvtx.TxOps) (uint64, error) {
	// Seek to the last node key.
	it := tx.Iterate(ctx, nodePrefix, true, true)
	defer it.Close()
	if err := it.Seek(nil); err != nil {
		return 0, err
	}
	if !it.Valid() {
		return 0, it.Err()
	}

	// Its ID is the largest.
	id, _, ok := parseNodeKey(it.Key())
	if !ok {
		return 0, errors.Errorf("node graph: invalid node key %x", it.Key())
	}
	return id, nil
}

// deleteIfEmpty deletes a node that has no edges and is not pinned.
//
// One seek at the node's out range covers both ranges, since in keys sort
// after out keys, and reads through the write buffer.
func (qs *QuadStore) deleteIfEmpty(ctx context.Context, id uint64) error {
	// Keep a node with edges or a pin.
	live, err := qs.hasEdges(ctx, id)
	if err != nil || live || qs.isPinned(id) {
		return err
	}
	meta, err := qs.readMeta(ctx, id)
	if err != nil || meta == nil || meta.GetPinned() {
		return err
	}

	// Delete its meta and its ID map entry.
	if err := qs.tx.Delete(ctx, metaKey(id)); err != nil {
		return err
	}
	return qs.tx.Delete(ctx, valueKey(meta.GetValue().ToNative().String()))
}

// hasEdges reports whether a node has an out or in key.
func (qs *QuadStore) hasEdges(ctx context.Context, id uint64) (bool, error) {
	// Seek to the first key past the node's meta.
	node := nodeKey(make([]byte, 0, nodeKeyLen), id)
	it := qs.tx.Iterate(ctx, node, true, false)
	defer it.Close()
	if err := it.Seek(rangeKey(id, tagOut)); err != nil {
		return false, err
	}

	// Any key still under the node is an edge.
	if !it.Valid() {
		return false, it.Err()
	}
	return bytes.HasPrefix(it.Key(), node), nil
}
