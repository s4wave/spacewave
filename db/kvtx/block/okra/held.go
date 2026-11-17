package kvtx_block_okra

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
)

// heldWrite is a cursor write held by a tree that defers its cursor writes: a
// materialized value, or a delete.
type heldWrite struct {
	ref    *block.BlockRef
	isBlob bool
	del    bool
}

// DeferCursorWrites makes a write transaction hold the entries SetCursorAtKey,
// Delete and DeleteCursorAtKey write, and apply them as one batch on Flush or
// Commit, so a key written many times, or many keys in one page, rebuild the
// page and its path to the root once. Each held value is written to the
// staging store when it is set, as before; only the page rebuild waits.
//
// Exists and GetCursorAtKey answer from the held entries. Every other method
// applies them first, so its result is the one an immediate write gives. A
// caller that reads the tree's root cursor, or the block holding it, calls
// Flush first. A read transaction ignores the call.
func (t *Tx) DeferCursorWrites() {
	if t.write {
		t.deferCursors = true
	}
}

// Held returns the number of held cursor writes.
func (t *Tx) Held() int {
	return len(t.held)
}

// Flush applies the held cursor writes as one batch. A failed batch can leave
// the tree partly written, so its error sticks: this and every later Flush,
// and so every read that applies held writes and Commit, return it, and the
// caller discards the transaction.
func (t *Tx) Flush(ctx context.Context) error {
	// A failed batch left the tree partly written.
	if t.flushErr != nil {
		return t.flushErr
	}

	// A tree that holds nothing has nothing to apply.
	if len(t.held) == 0 {
		return nil
	}

	// Build the held writes as one batch in key order.
	ops := make([]batchOp, 0, len(t.held))
	for key, h := range t.held {
		op := batchOp{key: []byte(key), del: h.del}
		if !h.del {
			op.entry = &BuildEntry{Key: op.key, ValueRef: h.ref, ValueIsBlob: h.isBlob}
		}
		ops = append(ops, op)
	}
	slices.SortFunc(ops, func(a, b batchOp) int { return bytes.Compare(a.key, b.key) })

	// Apply the batch, and drop the held writes once it is written.
	if err := t.applyBatch(ctx, ops); err != nil {
		t.flushErr = err
		return err
	}
	t.held = nil
	return nil
}

// hold records a cursor write to key.
func (t *Tx) hold(key []byte, h heldWrite) {
	if t.held == nil {
		t.held = make(map[string]heldWrite)
	}
	t.held[string(key)] = h
}

// heldAt returns the held write to key, if any.
func (t *Tx) heldAt(key []byte) (heldWrite, bool) {
	h, ok := t.held[string(key)]
	return h, ok
}

// heldCursor returns a cursor at a held value, in a private transaction over
// the staging store the value was written to.
func (t *Tx) heldCursor(ctx context.Context, h heldWrite) *block.Cursor {
	cursor := stagedCursor(ctx, t.bcs)
	cursor.SetRefAtCursor(h.ref, true)
	return cursor
}
