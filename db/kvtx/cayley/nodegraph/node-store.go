package kvtx_cayley_nodegraph

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// nodeStore owns node identity and reversible writes in one transaction.
// Its lock, allocation mark and undo buffers live for the transaction's graph.
type nodeStore struct {
	// tx holds the graph keys, including this transaction's held writes.
	tx kvtx.TxOps

	// idMtx guards nextID and pinned.
	idMtx sync.Mutex
	// nextID is the next node ID to allocate, which the tree's next ID key
	// also holds, or 0 before loadNextID.
	nextID uint64
	// pinned holds the nodes this transaction has seen pinned.
	pinned map[uint64]struct{}

	// undo holds the inverse of each write of the running ApplyDeltas call,
	// in order, and undoPins holds the nodes the call marked pinned. Only
	// the one writer touches them.
	undo     []undoEntry
	undoPins []uint64
}

// lookupID returns the ID of a value's text, or 0 when it has none.
func (qs *nodeStore) lookupID(ctx context.Context, text string) (uint64, error) {
	// Read the ID map entry. A missing entry means no node.
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
func (qs *nodeStore) readMeta(ctx context.Context, id uint64) (*NodeMeta, error) {
	// Read the meta key. A missing key means no node.
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

// nameOfID returns the value of a node ID.
func (qs *nodeStore) nameOfID(ctx context.Context, id uint64) (quad.Value, error) {
	meta, err := qs.readMeta(ctx, id)
	if err != nil || meta == nil {
		return nil, err
	}
	return meta.GetValue().ToNative(), nil
}

// undoEntry restores one key a running ApplyDeltas call wrote: it sets the key
// back to prev when the key held prev, and deletes the key otherwise.
type undoEntry struct {
	key  []byte
	prev []byte
	had  bool
}

// set writes a key that held prev when had is set, and records its undo.
func (qs *nodeStore) set(ctx context.Context, key, value, prev []byte, had bool) error {
	if err := qs.tx.Set(ctx, key, value); err != nil {
		return err
	}
	qs.undo = append(qs.undo, undoEntry{key: key, prev: prev, had: had})
	return nil
}

// del deletes a key that held prev, and records its undo.
func (qs *nodeStore) del(ctx context.Context, key, prev []byte) error {
	if err := qs.tx.Delete(ctx, key); err != nil {
		return err
	}
	qs.undo = append(qs.undo, undoEntry{key: key, prev: prev, had: true})
	return nil
}

// rollback undoes the running call's writes, newest first, and forgets the
// pins it marked, so a later pin in the transaction writes its meta again.
func (qs *nodeStore) rollback(ctx context.Context) error {
	// Forget the pins, then restore each key's prior value.
	qs.idMtx.Lock()
	for _, id := range qs.undoPins {
		delete(qs.pinned, id)
	}
	qs.idMtx.Unlock()
	for _, e := range slices.Backward(qs.undo) {

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

// createNode allocates a node for v and writes its ID map entry and meta.
func (qs *nodeStore) createNode(ctx context.Context, v quad.Value, pinned bool) (uint64, error) {
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
func (qs *nodeStore) pin(ctx context.Context, id uint64) error {
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
func (qs *nodeStore) isPinned(id uint64) bool {
	qs.idMtx.Lock()
	defer qs.idMtx.Unlock()
	_, ok := qs.pinned[id]
	return ok
}

// markPinned records that id is pinned.
func (qs *nodeStore) markPinned(id uint64) {
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
// next ID key before the caller writes any key of the node. The key goes to the
// transaction's write buffer, so a run of allocations costs one tree write.
// Writing the key first means a call that fails after the allocation, and a
// caller that commits anyway, still leaves the ID behind the key.
func (qs *nodeStore) allocID(ctx context.Context) (uint64, error) {
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
func (qs *nodeStore) nodeEstimate(ctx context.Context) (int64, error) {
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
func (qs *nodeStore) loadNextID(ctx context.Context) error {
	// Loaded once per store.
	if qs.nextID != 0 {
		return nil
	}

	// Read the stored mark.
	v, found, err := qs.tx.Get(ctx, nextIDKey)
	if err != nil {
		return err
	}

	// Start a tree without the next ID key at 1 once one seek per prefix
	// shows the tree empty.
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
func (qs *nodeStore) deleteIfEmpty(ctx context.Context, id uint64) error {
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
func (qs *nodeStore) hasKeyUnder(ctx context.Context, prefix []byte) (bool, error) {
	it := qs.tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if !it.Next() {
		return false, it.Err()
	}
	return true, nil
}

// hasEdges reports whether a node has an out or in key.
func (qs *nodeStore) hasEdges(ctx context.Context, id uint64) (bool, error) {
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

// containsQuad reports whether the quad's out key exists.
func (qs *nodeStore) containsQuad(ctx context.Context, q quadRef) (bool, error) {
	out, _ := edgeKeys(q)
	return qs.tx.Exists(ctx, out)
}

// clearUndo releases the running call's saved values and retains its buffers.
func (qs *nodeStore) clearUndo() {
	clear(qs.undo)
	qs.undo, qs.undoPins = qs.undo[:0], qs.undoPins[:0]
}
