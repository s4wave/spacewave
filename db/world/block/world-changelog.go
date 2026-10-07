package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// queueWorldChange adds a world change to the apply queue and updates the seqno.
// expects rmtx to be locked
// returns nil, nil if changelog disabled
func (t *WorldState) queueWorldChange(ctx context.Context, w *WorldChange) (*block.Cursor, error) {
	// Reject an empty change or a read-only state.
	if w == nil {
		return nil, world.ErrEmptyOp
	}
	if !t.write {
		return nil, tx.ErrNotWrite
	}

	// Load the World root.
	r, err := t.GetRoot(ctx)
	if err != nil {
		return nil, err
	}

	// Stage the change block unless the changelog is disabled.
	var changeBcs *block.Cursor
	if !r.GetLastChangeDisable() {
		changeBcs = t.bcs.Detach(false)
		changeBcs.SetBlock(w, true)
	}
	t.pendingChanges = append(t.pendingChanges, changeBcs)

	// Update the sequence number and return the change cursor.
	t.updateSeqno(r)
	return changeBcs, nil
}

// updateSeqno computes the latest sequence number and updates t.seqno.
func (t *WorldState) updateSeqno(r *World) {
	// estimate next sequence number
	currSeqno := r.GetLastChange().GetSeqno()
	nextSeqno := currSeqno + uint64(len(t.pendingChanges))
	t.seqnoBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if t.seqno != nextSeqno {
			t.seqno = nextSeqno
			broadcast()
		}
	})
}

// flushWorldChanges flushes the queued world changes to the log.
// if an error is returned, the changelog is likely now in a broken state.
func (t *WorldState) flushWorldChanges(ctx context.Context, w *World) error {
	// Start the flush trace.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/flush-world-changes")
	defer task.End()

	// Reject a read-only state.
	if !t.write {
		return tx.ErrNotWrite
	}

	// Take the pending change queue.
	queue := t.pendingChanges
	t.pendingChanges = nil

	// Advance a disabled changelog's seqno without recording entries.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/flush-world-changes/get-root")
	r, err := t.GetRoot(taskCtx)
	subtask.End()
	if err != nil {
		return err
	}
	if r.GetLastChangeDisable() {
		if r.LastChange == nil {
			r.LastChange = &ChangeLogLL{}
		}
		r.LastChange.Seqno += uint64(len(queue))
		t.bcs.FollowSubBlock(3).SetBlock(r.LastChange, true)
		return nil
	}

	// Walk the pending change queue.
	i := 0
	for i < len(queue) {
		chiWc, err := UnmarshalWorldChange(ctx, queue[i])
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

		// append change set
		changeSet := queue[i:x]
		taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/flush-world-changes/append-entry")
		_, err = t.appendChangelogEntry(taskCtx, w, changeSet)
		subtask.End()
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
	// Start the append trace.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/append-changelog-entry")
	defer task.End()

	// Return the last-change cursor when there is nothing to append.
	lastChangeBcs := t.bcs.FollowSubBlock(3)
	if len(changesBcs) == 0 {
		return lastChangeBcs, nil
	}

	// Read the object-tree size for the changelog filter.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/append-changelog-entry/object-tree-size")
	objSize, err := t.objTree.Size(taskCtx)
	subtask.End()
	if err != nil {
		return nil, err
	}

	// Seal the segment when this entry starts a new one. The World keeps the
	// sealed head as the previous segment and drops the segment before it.
	lastChange := w.GetLastChange()
	seqno := lastChange.GetSeqno() + 1
	prevBcs := lastChangeBcs
	switch {
	case lastChange.IsEmpty():
		prevBcs = nil
	case seqno%ChangeLogSegmentLen == 1:
		t.bcs.SetRef(7, lastChangeBcs.Detach(true))
		prevBcs = nil
	}

	// Append the changelog node and store it as the last change.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/append-changelog-entry/append-change-log")
	lc, err := AppendChangeLogLL(taskCtx, objSize, seqno, lastChangeBcs, prevBcs, changesBcs)
	subtask.End()
	if err != nil {
		return nil, err
	}
	w.LastChange = lc
	return lastChangeBcs, nil
}
