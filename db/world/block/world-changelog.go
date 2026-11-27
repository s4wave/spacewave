package world_block

import (
	"context"
	"maps"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// quadKey identifies a graph quad by its canonical Cayley string fields.
type quadKey struct {
	// subj is the quad subject as the graph stores it.
	subj string
	// pred is the quad predicate as the graph stores it.
	pred string
	// obj is the quad object as the graph stores it.
	obj string
	// label is the quad label as the graph stores it.
	label string
}

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

	// Stage the change block unless the changelog is disabled, counting the
	// seqnos the flush will add for it.
	var changeBcs *block.Cursor
	disabled := r.GetLastChangeDisable()
	if !disabled {
		changeBcs = t.bcs.Detach(false)
		changeBcs.SetBlock(w, true)
	}
	if disabled || len(t.pendingChanges) == 0 || w.GetChangeType() != t.pendingType {
		t.pendingEntries++
	}
	t.pendingChanges = append(t.pendingChanges, changeBcs)
	t.pendingType = w.GetChangeType()
	t.noteTypeChange(w)
	t.noteChange(w)

	// Update the sequence number, wake the waiters of the keys the change
	// references and return the change cursor.
	t.updateSeqno(r)
	t.revs.wake(w.GetKey(), w.GetNewKey())
	return changeBcs, nil
}

// noteChange records w in the transaction's ChangeSet, collapsing to unknown
// once the distinct keys and quads exceed changeDiffLimit.
func (t *WorldState) noteChange(w *WorldChange) {
	// Stop recording once the transaction already exceeded changeDiffLimit.
	if t.changeUnknown {
		return
	}

	// Record any touched object keys, including both ends of a rename.
	if key := w.GetKey(); key != "" {
		if t.changeKeys == nil {
			t.changeKeys = make(map[string]struct{})
		}
		t.changeKeys[key] = struct{}{}
	}
	if newKey := w.GetNewKey(); newKey != "" {
		if t.changeKeys == nil {
			t.changeKeys = make(map[string]struct{})
		}
		t.changeKeys[newKey] = struct{}{}
	}

	// Record any touched graph quad spelled as the graph stores it.
	if q := w.GetQuad(); q != nil {
		cq, err := world.GraphQuadToCayleyQuad(world.QuadToGraphQuad(q), false)
		if err != nil {
			t.changeUnknown = true
			t.changeKeys, t.changeQuads = nil, nil
			return
		}
		gq := world.CayleyQuadToGraphQuad(cq)
		if t.changeQuads == nil {
			t.changeQuads = make(map[quadKey]struct{})
		}
		t.changeQuads[quadKey{
			subj:  gq.GetSubject(),
			pred:  gq.GetPredicate(),
			obj:   gq.GetObj(),
			label: gq.GetLabel(),
		}] = struct{}{}
	}

	// Collapse to unknown once the change exceeds the diff bound.
	if len(t.changeKeys)+len(t.changeQuads) > changeDiffLimit {
		t.changeUnknown = true
		t.changeKeys, t.changeQuads = nil, nil
	}
}

// takeChanges returns the ChangeSet accumulated since the last take, Commit,
// Discard, or SetBlockTransaction and clears the accumulator.
func (t *WorldState) takeChanges() *world.ChangeSet {
	// Reset the accumulator on exit and return unknown if it overflowed.
	defer t.resetChanges()
	if t.changeUnknown {
		return world.NewUnknownChangeSet()
	}

	// Materialize the sorted keys and distinct quads.
	var keys []string
	if len(t.changeKeys) != 0 {
		keys = slices.Sorted(maps.Keys(t.changeKeys))
	}
	var quads []world.GraphQuad
	if len(t.changeQuads) != 0 {
		quads = make([]world.GraphQuad, 0, len(t.changeQuads))
		for q := range t.changeQuads {
			quads = append(quads, world.NewGraphQuad(q.subj, q.pred, q.obj, q.label))
		}
	}
	return &world.ChangeSet{Keys: keys, Quads: quads}
}

// resetChanges clears the transaction's accumulated ChangeSet state.
func (t *WorldState) resetChanges() {
	t.changeKeys = nil
	t.changeQuads = nil
	t.changeUnknown = false
}

// updateSeqno computes the latest sequence number, updates t.seqno and wakes
// the seqno waiters when it moved.
func (t *WorldState) updateSeqno(r *World) {
	// Add the seqnos the pending changes will reach.
	currSeqno := r.GetLastChange().GetSeqno()
	nextSeqno := currSeqno + t.pendingEntries
	t.seqnoBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if t.seqno != nextSeqno {
			t.seqno = nextSeqno
			broadcast()
		}
	})
}

// SetChangelogDisabled turns the changelog off or on and reports whether the
// World changed. Expects rmtx to be locked. An operation may call it on the
// WorldState it is applied to, so every replay applies the same setting.
//
// Changes queued before the switch are recorded under the previous setting.
// Disabling drops the recorded history. Enabling starts the log at the current
// seqno without history.
func (t *WorldState) SetChangelogDisabled(ctx context.Context, disable bool) (bool, error) {
	// Reject a read-only state.
	if !t.write {
		return false, tx.ErrNotWrite
	}

	// Load the World root and skip a matching setting.
	r, err := t.GetRoot(ctx)
	if err != nil {
		return false, err
	}
	if r.GetLastChangeDisable() == disable {
		return false, nil
	}

	// Record the queued changes under the previous setting.
	if err := t.flushWorldChanges(ctx, r); err != nil {
		return false, err
	}

	// Keep only the seqno: its bare head entry marks the start of history.
	r.LastChangeDisable = disable
	r.LastChange = &ChangeLogLL{Seqno: r.GetLastChange().GetSeqno()}
	r.PrevChanges = nil
	t.bcs.ClearRef(7)
	lastChangeBcs := t.bcs.FollowSubBlock(3)
	lastChangeBcs.ClearAllRefs()
	lastChangeBcs.SetBlock(r.LastChange, true)
	return true, nil
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
	t.pendingChanges, t.pendingEntries = nil, 0

	// Load the root and add the queue's type edges to its counts. The root is
	// written with the changelog head below, which every non-empty queue sets.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/flush-world-changes/get-root")
	r, err := t.GetRoot(taskCtx)
	subtask.End()
	if err != nil {
		return err
	}
	t.flushTypeCounts(r)

	// Advance a disabled changelog's seqno without recording entries.
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
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/append-changelog-entry/append-change-log")
	lc, err := AppendChangeLogLL(taskCtx, seqno, lastChangeBcs, prevBcs, changesBcs)
	subtask.End()
	if err != nil {
		return nil, err
	}
	w.LastChange = lc
	return lastChangeBcs, nil
}
