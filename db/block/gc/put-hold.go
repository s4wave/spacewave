package block_gc

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// PutHold is what Land does with a block the batch gives birth to and gives
// no holder, a put no root, parent or stage holds, which the collector would
// free once its record ages past the grace period.
type PutHold struct {
	// Mode refuses such a Land with block.ErrUnheldPut in HeldEnforce,
	// counts the blocks in HeldObserve, and checks nothing in HeldOff.
	Mode HeldMode
	// Observed receives, in HeldObserve, the count of such blocks under
	// NotHeldPut when it is nonzero, before the Land's transaction commits.
	// Nil drops it.
	Observed func(notHeld [NotHeldPut + 1]uint64, err error)
	// Partial reports that the batch copies part of a tree another store
	// holds, as a cache fill does: a born block or a refilled hole may
	// name a child the generation hides, which Land counts as it counts an
	// absent child instead of refusing. A root onto a hidden block is still
	// refused.
	Partial bool
}

// requirePutsHeld refuses, counts or passes, as puts.Mode says, the born
// blocks the batch gives no holder once the children and roots are counted
// and whose count records none from before. A born block that a stored or
// refilled entry of the batch names is held: that parent's list counted it
// when the parent was born, through an up a child stored after its
// parent nets at the settle, so its count is only not recorded yet.
// It reads nothing: the holders are the batch's, in memory.
func (l *lander) requirePutsHeld(puts PutHold) error {
	// Off checks nothing.
	if puts.Mode == HeldOff {
		return nil
	}

	// Find the unheld born blocks, refusing the first while enforcing.
	var counts [NotHeldPut + 1]uint64
	var listed map[string]struct{}
	for _, b := range l.order {
		if b.holders != 0 || b.counted {
			continue
		}
		if listed == nil {
			listed = l.listedNames()
		}
		if _, ok := listed[string(BlockKey(b.ref))]; ok {
			continue
		}
		if puts.Mode == HeldEnforce {
			return errors.Wrap(block.ErrUnheldPut, b.ref.MarshalString())
		}
		counts[NotHeldPut]++
	}
	if puts.Observed != nil && counts[NotHeldPut] != 0 {
		puts.Observed(counts, nil)
	}
	return nil
}

// listedNames returns the block keys of the children the batch's stored and
// refilled entries name, whose lists hold them already.
func (l *lander) listedNames() map[string]struct{} {
	names := make(map[string]struct{})
	for _, entries := range [][]*block.PutBatchEntry{l.stored, l.refills} {
		for _, entry := range entries {
			for _, ref := range entry.Refs {
				names[string(BlockKey(ref))] = struct{}{}
			}
		}
	}
	return names
}
