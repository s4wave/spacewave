package block_gc

import (
	"context"
	"slices"

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
	// reference a child the generation hides, which Land counts as it
	// counts an absent child instead of refusing. A root onto a hidden
	// block is still refused.
	Partial bool
}

// requirePutsHeld refuses, counts or passes, as puts.Mode says, the born blocks
// the batch gives no holder once the children and roots are counted and whose
// count records none from before.
//
// Invariant: a born block passes only when a holder the collector counts
// names it. A stored or refilled entry of the batch that references it is
// one: that parent's list counted it when the parent was born, through an up
// a child stored after its parent nets at the settle, or through the recount
// a switch-on runs, so its count is only not recorded yet. A copied entry of
// a stored block, parked or not, is one only through the list the store
// holds for it, which the put leaves as it was and which the counts and the
// recount read; its copied refs prove nothing, so the check reads that list.
// A staged copy relies on this: the stage gives no root to a block another
// entry of the slice names, so the child of a stored block put again is held
// by that block's list alone.
//
// Cost: nothing when every born block has a holder, the usual case; otherwise
// one child list read per copied entry whose refs name an unheld block.
func (l *lander) requirePutsHeld(ctx context.Context, puts PutHold) error {
	// Off checks nothing.
	if puts.Mode == HeldOff {
		return nil
	}

	// Gather the born blocks the batch gives no holder.
	unheld := make(map[string]*block.BlockRef)
	for _, b := range l.order {
		if b.holders == 0 && !b.counted {
			unheld[string(BlockKey(b.ref))] = b.ref
		}
	}
	if len(unheld) == 0 {
		return nil
	}

	// Drop those a listed parent holds, then refuse the first left while
	// enforcing.
	if err := l.dropListed(ctx, unheld); err != nil {
		if puts.Mode == HeldEnforce {
			return err
		}
		if puts.Observed != nil {
			puts.Observed([NotHeldPut + 1]uint64{}, err)
		}
		return nil
	}
	var counts [NotHeldPut + 1]uint64
	for _, b := range l.order {
		if _, ok := unheld[string(BlockKey(b.ref))]; !ok {
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

// dropListed deletes from unheld each block a list the batch keeps names: the
// refs of its stored and refilled entries, and the stored list of each copied
// entry of a stored block whose refs name an unheld block.
func (l *lander) dropListed(ctx context.Context, unheld map[string]*block.BlockRef) error {
	// Entries whose refs the batch reads from content list them.
	for _, entries := range [][]*block.PutBatchEntry{l.stored, l.refills} {
		for _, entry := range entries {
			for _, ref := range entry.Refs {
				delete(unheld, string(BlockKey(ref)))
			}
		}
	}

	// A copied entry holds a child only through its stored list.
	for _, entry := range l.kept {
		if len(unheld) == 0 {
			return nil
		}
		if !slices.ContainsFunc(entry.Refs, func(ref *block.BlockRef) bool {
			_, ok := unheld[string(BlockKey(ref))]
			return ok
		}) {
			continue
		}
		children, _, err := l.rec.GetChildren(ctx, entry.Ref)
		if err != nil {
			return err
		}
		for _, child := range children {
			delete(unheld, string(BlockKey(child)))
		}
	}
	return nil
}
