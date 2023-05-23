package world_block

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// queueWorldChange adds a world change to the apply queue.
// returns nil, nil if changelog disabled
func (t *WorldState) queueWorldChange(ctx context.Context, w *WorldChange) (*block.Cursor, error) {
	if w == nil {
		return nil, world.ErrEmptyOp
	}
	if !t.write {
		return nil, tx.ErrNotWrite
	}
	r, err := t.GetRoot(ctx)
	if err != nil {
		return nil, err
	}
	if r.GetLastChangeDisable() {
		t.pendingChanges = append(t.pendingChanges, nil)
		return nil, nil
	}

	changeBcs := t.bcs.Detach(false)
	changeBcs.SetBlock(w, true)
	t.pendingChanges = append(t.pendingChanges, changeBcs)
	return changeBcs, nil
}

// flushWorldChanges flushes the queued world changes to the log.
// if an error is returned, the changelog is likely now in a broken state.
func (t *WorldState) flushWorldChanges(ctx context.Context, w *World) error {
	if !t.write {
		return tx.ErrNotWrite
	}

	queue := t.pendingChanges
	t.pendingChanges = nil

	r, err := t.GetRoot(ctx)
	if err != nil {
		return err
	}
	if r.LastChange == nil {
		r.LastChange = &ChangeLogLL{}
	}
	lastChangeBcs := t.bcs.FollowSubBlock(3)
	if r.GetLastChangeDisable() {
		r.LastChange.Seqno += uint64(len(queue))
		lastChangeBcs.SetBlock(r.LastChange, true)
		return nil
	}

	i := 0
	for i < len(queue) {
		chi := queue[i]
		if chi == nil {
			continue
		}

		chiWc, err := UnmarshalWorldChange(ctx, chi)
		if err != nil {
			return err
		}

		x := i + 1

		// batch sequential changes of identical type
		for x < len(queue) {
			chx := queue[x]
			chxWc, err := UnmarshalWorldChange(ctx, chx)
			if err != nil {
				return err
			}
			if chxWc.GetChangeType() != chiWc.GetChangeType() {
				break
			}
			x++
		}
		changeSet := queue[i:x]
		_, err = t.appendChangelogEntry(ctx, w, changeSet)
		if err != nil {
			return err
		}
		i += len(changeSet)
	}

	return nil
}

// appendChangelogEntry sets change as the new LastChange field.
// returns the block cursor containing HEAD ChangeLogLL (sub-block)
// changes must all have the same change type.
func (t *WorldState) appendChangelogEntry(ctx context.Context, w *World, changesBcs []*block.Cursor) (*block.Cursor, error) {
	lastChangeBcs := t.bcs.FollowSubBlock(3)
	if len(changesBcs) == 0 {
		return lastChangeBcs, nil
	}
	objSize, err := t.objTree.Size(ctx)
	if err != nil {
		return nil, err
	}
	lc, err := AppendChangeLogLL(ctx, objSize, lastChangeBcs, lastChangeBcs, changesBcs)
	if err != nil {
		return nil, err
	}
	w.LastChange = lc
	return lastChangeBcs, nil
}
