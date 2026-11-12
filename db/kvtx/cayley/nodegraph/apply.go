package kvtx_cayley_nodegraph

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
)

// quadText identifies a quad by its values' text, with "" for no label.
type quadText [4]string

// textOf returns the text of a quad's values.
func textOf(q quad.Quad) quadText {
	var t quadText
	for i, d := range quad.Directions {
		if v := q.Get(d); v != nil {
			t[i] = valueText(v)
		}
	}
	return t
}

// ApplyDeltas adds and removes quads.
//
// Every delta is checked before any key is written, so a rejected call
// changes nothing. A write that fails later in the call is undone with the
// call's other writes, all but the next ID key, which only grows, so the
// transaction holds no quad with one of its two keys and no node without its
// ID map entry even when its owner commits it after the error. A node left
// without edges is deleted with its ID map entry in the same call, unless it
// is pinned as a predicate or label.
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

	// Write the accepted deltas, undoing the call's writes when one fails.
	err := b.write(ctx, in, apply)
	if err != nil {
		if rerr := qs.rollback(context.WithoutCancel(ctx)); rerr != nil {
			return errors.Wrapf(err, "node graph: undo failed: %v", rerr)
		}
	}
	clear(qs.undo)
	qs.undo, qs.undoPins = qs.undo[:0], qs.undoPins[:0]
	return err
}

// write writes the accepted deltas, then deletes the nodes they left empty.
func (b *deltaBatch) write(ctx context.Context, in []graph.Delta, apply []bool) error {
	emptied := make(map[uint64]struct{})
	for i, d := range in {
		if !apply[i] {
			continue
		}
		var err error
		if d.Action == graph.Add {
			err = b.add(ctx, d.Quad)
		} else {
			err = b.remove(ctx, d.Quad, emptied)
		}
		if err != nil {
			return err
		}
	}
	for id := range emptied {
		if err := b.qs.deleteIfEmpty(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// undoEntry restores one key a running ApplyDeltas call wrote: it sets the key
// back to prev when the key held it, and deletes it otherwise.
type undoEntry struct {
	key  []byte
	prev []byte
	had  bool
}

// set writes a key that held prev when had is set, and records its undo.
func (qs *QuadStore) set(ctx context.Context, key, value, prev []byte, had bool) error {
	if err := qs.tx.Set(ctx, key, value); err != nil {
		return err
	}
	qs.undo = append(qs.undo, undoEntry{key: key, prev: prev, had: had})
	return nil
}

// del deletes a key that held prev, and records its undo.
func (qs *QuadStore) del(ctx context.Context, key, prev []byte) error {
	if err := qs.tx.Delete(ctx, key); err != nil {
		return err
	}
	qs.undo = append(qs.undo, undoEntry{key: key, prev: prev, had: true})
	return nil
}

// rollback undoes the running call's writes, newest first, and forgets the
// pins it marked, so a later pin in the transaction writes its meta again.
func (qs *QuadStore) rollback(ctx context.Context) error {
	// Forget the pins, then restore each key's prior value.
	qs.idMtx.Lock()
	for _, id := range qs.undoPins {
		delete(qs.pinned, id)
	}
	qs.idMtx.Unlock()
	for i := len(qs.undo) - 1; i >= 0; i-- {
		e := qs.undo[i]
		var err error
		if e.had {
			err = qs.tx.Set(ctx, e.key, e.prev)
		} else {
			err = qs.tx.Delete(ctx, e.key)
		}
		if err != nil {
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
	text := valueText(v)
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
		b.ids[valueText(v)] = id
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
	if err := b.qs.set(ctx, out, []byte{}, nil, false); err != nil {
		return err
	}
	return b.qs.set(ctx, in, []byte{}, nil, false)
}

// remove deletes the out and in keys of a quad that exists, and adds its
// endpoints to emptied for the liveness check.
func (b *deltaBatch) remove(ctx context.Context, q quad.Quad, emptied map[uint64]struct{}) error {
	// Resolve the quad, which the check found, to its node IDs.
	qr, ok, err := b.resolve(ctx, q)
	if err != nil {
		return err
	}
	if !ok {
		return errors.Errorf("node graph: removed quad %v has no node", q)
	}

	// Delete the edge under both endpoints.
	out, in := edgeKeys(qr)
	if err := b.qs.del(ctx, out, []byte{}); err != nil {
		return err
	}
	if err := b.qs.del(ctx, in, []byte{}); err != nil {
		return err
	}

	// Queue each endpoint once for the liveness check.
	emptied[qr.s] = struct{}{}
	emptied[qr.o] = struct{}{}
	return nil
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
	if err := qs.set(ctx, valueKey(valueText(v)), encodeID(id), nil, false); err != nil {
		return 0, err
	}
	if err := qs.set(ctx, metaKey(id), meta, nil, false); err != nil {
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
		prev, err := meta.MarshalVT()
		if err != nil {
			return err
		}
		meta.Pinned = true
		data, err := meta.MarshalVT()
		if err != nil {
			return err
		}
		if err := qs.set(ctx, metaKey(id), data, prev, true); err != nil {
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
	if _, ok := qs.pinned[id]; !ok {
		qs.pinned[id] = struct{}{}
		qs.undoPins = append(qs.undoPins, id)
	}
}

// allocID returns a new node ID, the tree's next ID, and advances the tree's
// next ID key before the caller writes any key of the node. The key goes to
// the transaction's write buffer, so a run of allocations costs one tree
// write; writing it first means a call that fails after the allocation, and a
// caller that commits anyway, still leaves the ID behind the key.
func (qs *QuadStore) allocID(ctx context.Context) (uint64, error) {
	// Load the next ID under the lock on first use.
	qs.idMtx.Lock()
	defer qs.idMtx.Unlock()
	if err := qs.loadNextID(ctx); err != nil {
		return 0, err
	}

	// Store the advanced mark, then take the ID.
	id := qs.nextID
	if err := qs.tx.Set(ctx, nextIDKey, encodeID(id+1)); err != nil {
		return 0, err
	}
	qs.nextID++
	return id, nil
}

// nodeEstimate returns the number of node IDs allocated, an upper bound on the
// live nodes: a deleted node leaves a gap.
func (qs *QuadStore) nodeEstimate(ctx context.Context) (int64, error) {
	qs.idMtx.Lock()
	defer qs.idMtx.Unlock()
	if err := qs.loadNextID(ctx); err != nil {
		return 0, err
	}
	return int64(qs.nextID - 1), nil //nolint:gosec // IDs stay far below 1<<63.
}

// loadNextID sets nextID on first use from the tree's next ID key: one read
// per store, through the write buffer without flushing it. A tree without the
// key never held a node, so its next ID is 1. The caller holds idMtx.
//
// A tree with nodes but no next ID key is refused rather than repaired: its
// highest live ID is no bound on the IDs deleted nodes held, so counting on
// from it could allocate one of them again.
func (qs *QuadStore) loadNextID(ctx context.Context) error {
	// Loaded once per store.
	if qs.nextID != 0 {
		return nil
	}

	// Read the stored mark.
	v, found, err := qs.tx.Get(ctx, nextIDKey)
	if err != nil {
		return err
	}

	// Start a tree without it at 1, once one seek per prefix shows it empty.
	if !found {
		for _, prefix := range [][]byte{nodePrefix, valuePrefix} {
			held, err := qs.hasKeyUnder(ctx, prefix)
			if err != nil {
				return err
			}
			if held {
				return ErrMissingNextID
			}
		}
		qs.nextID = 1
		return nil
	}
	id, ok := decodeID(v)
	if !ok || id == 0 {
		return errors.Errorf("node graph: invalid next ID %x", v)
	}
	qs.nextID = id
	return nil
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
	prev, err := meta.MarshalVT()
	if err != nil {
		return err
	}
	if err := qs.del(ctx, metaKey(id), prev); err != nil {
		return err
	}
	return qs.del(ctx, valueKey(valueText(meta.GetValue().ToNative())), encodeID(id))
}

// hasKeyUnder reports whether the tree holds a key with prefix.
func (qs *QuadStore) hasKeyUnder(ctx context.Context, prefix []byte) (bool, error) {
	it := qs.tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if !it.Next() {
		return false, it.Err()
	}
	return true, nil
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
