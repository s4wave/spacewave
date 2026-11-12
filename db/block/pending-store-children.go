package block

import (
	"context"

	"github.com/pkg/errors"
)

// ErrMissingChild refuses a write to the inner store of a block that names a
// child no store holds: written beside it, kept, or stored. Storing it would
// leave a stored World that cannot be read; the refused write moves nothing,
// so the roots keep naming what they held.
var ErrMissingChild = errors.New("block names a child that is neither written, kept nor stored")

// MissingChildError is the refusal of one write: Op names the write, Block the
// block it refused and Child the child of Block no store holds. It matches
// ErrMissingChild.
type MissingChildError struct {
	// Op names the refused write, such as a Flush or a SetRoots.
	Op string
	// Block is the refused block.
	Block *BlockRef
	// Child is the child of Block no store holds.
	Child *BlockRef
}

// Error names the write, the block and its child.
func (e *MissingChildError) Error() string {
	return e.Op + ": block " + e.Block.MarshalString() + " names " + e.Child.MarshalString() + ": " + ErrMissingChild.Error()
}

// Is reports whether target is ErrMissingChild.
func (e *MissingChildError) Is(target error) bool {
	return target == ErrMissingChild
}

// checkChildren refuses batch with a MissingChildError, naming op, the block and
// its child by their block references, when a block of batch names a child
// that is neither in batch, kept, nor in the inner store. A kept child is
// either written by this write's caller or stored by an open Flush.
//
// Cost: one inner existence read of every distinct child the batch names
// that it neither writes nor keeps, the frontier between the kept pages and
// the stored ones.
func (s *PendingStore) checkChildren(ctx context.Context, op string, batch []*PutBatchEntry) error {
	// Gather the children neither written with the batch nor kept.
	seen := make(map[string]struct{}, len(batch))
	for _, e := range batch {
		seen[pendingKey(e.Ref)] = struct{}{}
	}
	var children, parents []*BlockRef
	s.mtx.RLock()
	for _, e := range batch {
		for _, ref := range e.Refs {
			if ref.GetEmpty() {
				continue
			}
			key := pendingKey(ref)
			if _, ok := seen[key]; ok || s.kept[key] != nil {
				continue
			}
			seen[key] = struct{}{}
			children, parents = append(children, ref), append(parents, e.Ref)
		}
	}
	s.mtx.RUnlock()

	// Refuse the batch when the inner store lacks one.
	if len(children) == 0 {
		return nil
	}
	found, err := s.StoreOps.GetBlockExistsBatch(ctx, children)
	if err != nil {
		return err
	}
	for i, ok := range found {
		if !ok {
			return &MissingChildError{Op: op, Block: parents[i], Child: children[i]}
		}
	}
	return nil
}
