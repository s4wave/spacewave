package block

import (
	"context"
	"math"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/tx"
)

// WriteAtRoots materializes selected subtrees in one encode/write pass. Every
// cursor must be a regular block in this transaction. The enclosing root and
// blocks outside these subtrees are not written. Discard on error, as with
// WriteAtRoot; a successful return does not add a separate durability fence.
func (t *Transaction) WriteAtRoots(ctx context.Context, roots []*Cursor) error {
	if t == nil {
		return tx.ErrNotWrite
	}
	if len(roots) == 0 {
		return ctx.Err()
	}
	if uint64(len(roots)) > math.MaxUint32 {
		return errors.New("too many subtree roots")
	}
	for _, root := range roots {
		if root == nil || root.t != t || root.pos.isSubBlock {
			return errors.New("subtree roots must be regular blocks in this transaction")
		}
	}

	// A zero-byte anchor joins existing cursor edges for this pass. It never
	// becomes a stored block and is removed after the write workers settle.
	anchor := roots[0].Detach(false)
	anchor.SetBlock(&writeRootsAnchor{}, true)
	defer func() {
		// Remove the anchor's adjacency once before detaching its handles.
		// Removing each edge separately would scan this large sibling list.
		t.mtx.Lock()
		defer t.mtx.Unlock()
		t.blockGraph.RemoveNode(anchor.pos.ID())
		anchor.clearRefHandles()
	}()
	for i, root := range roots {
		anchor.SetRef(uint32(i), root) //nolint:gosec // root count is bounded above.
	}
	_, _, err := t.WriteAtRoot(ctx, false, anchor)
	return err
}

// writeRootsAnchor groups cursor edges without introducing persisted content.
type writeRootsAnchor struct{}

func (*writeRootsAnchor) MarshalBlock() ([]byte, error) { return nil, nil }
func (*writeRootsAnchor) UnmarshalBlock([]byte) error   { return nil }
