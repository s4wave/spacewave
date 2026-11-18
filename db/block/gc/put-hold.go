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
}

// requirePutsHeld refuses, counts or passes, as puts.Mode says, the born
// blocks the batch gives no holder once the children and roots are counted
// and whose count records none from before.
// It reads nothing: the holders are the batch's, in memory.
func (l *lander) requirePutsHeld(puts PutHold) error {
	// Off checks nothing.
	if puts.Mode == HeldOff {
		return nil
	}

	// Find the unheld born blocks, refusing the first while enforcing.
	var counts [NotHeldPut + 1]uint64
	for _, b := range l.order {
		if b.holders != 0 || b.counted {
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
