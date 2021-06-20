package world_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
)

// queueWorldChange adds a world change to the apply queue.
// returns nil, nil if changelog disabled
func (t *WorldState) queueWorldChange(w *WorldChange) (*block.Cursor, error) {
	if w == nil {
		return nil, world.ErrEmptyOp
	}
	r, err := t.getRoot()
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
func (t *WorldState) flushWorldChanges(w *World) error {
	queue := t.pendingChanges
	t.pendingChanges = nil

	r, err := t.getRoot()
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
		chiWc, err := UnmarshalWorldChange(chi)
		if err != nil {
			return err
		}

		x := i + 1

		// batch sequential changes of identical type
		for x < len(queue) {
			chx := queue[x]
			chxWc, err := UnmarshalWorldChange(chx)
			if err != nil {
				return err
			}
			if chxWc.GetChangeType() != chiWc.GetChangeType() {
				break
			}
			x++
		}
		changeSet := queue[i:x]
		_, err = t.appendChangelogEntry(w, changeSet)
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
func (t *WorldState) appendChangelogEntry(w *World, changesBcs []*block.Cursor) (*block.Cursor, error) {
	lastChangeBcs := t.bcs.FollowSubBlock(3)
	if len(changesBcs) == 0 {
		return lastChangeBcs, nil
	}
	objSize, err := t.objTree.Size()
	if err != nil {
		return nil, err
	}
	lc, err := AppendChangeLogLL(objSize, lastChangeBcs, lastChangeBcs, changesBcs)
	if err != nil {
		return nil, err
	}
	w.LastChange = lc
	return lastChangeBcs, nil
}
