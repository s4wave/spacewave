package world_block

import "github.com/aperturerobotics/hydra/block"

// appendChangelogEntry sets change as the new LastChange field.
// returns the block cursor containing change (sub-block)
func (t *WorldState) appendChangelogEntry(w *World, change *WorldChange) (*block.Cursor, error) {
	lastChange := w.GetLastChange()
	lastChangeBcs := t.bcs.FollowSubBlock(3)
	if change == lastChange {
		return lastChangeBcs, nil
	}
	wBcs := lastChangeBcs.Detach(false)
	w.LastChange = change
	lastChangeBcs.ClearRef(2)
	_ = lastChange.AppendChange(wBcs, lastChangeBcs, change)
	return lastChangeBcs, nil
}
