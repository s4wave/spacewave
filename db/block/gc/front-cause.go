package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// FrontCause explains the collector's proof for one front entry.
type FrontCause uint8

const (
	// FrontHeldByHolders means the block's holder count is not zero.
	FrontHeldByHolders FrontCause = iota
	// FrontZeroUnqueued means a ripe listed block lacks queue, released and parked entries at its ripe sequence.
	FrontZeroUnqueued
	// FrontRipening means the up cursor has not reached the zero block's ripe sequence.
	FrontRipening
	// FrontDamaged means a ripe unlisted block at zero still has stored bytes.
	FrontDamaged
	// FrontPoppable means the front entry's free proof holds.
	FrontPoppable
	// FrontUnborn means the store holds neither bytes, a child list nor a hole record for a block at one holder.
	// The count cannot tell the implied holder of an unborn block from one real holder, so the reference scan settles it.
	FrontUnborn
	// frontCauses is the number of causes.
	frontCauses
)

// String returns the cause for an operator's diagnostic report.
func (c FrontCause) String() string {
	switch c {
	case FrontHeldByHolders:
		return "held by holders"
	case FrontZeroUnqueued:
		return "at zero, listed, not queued, released or parked"
	case FrontRipening:
		return "ripening"
	case FrontDamaged:
		return "damaged"
	case FrontPoppable:
		return "poppable"
	case FrontUnborn:
		return "absent, one holder, no list or hole"
	default:
		return "unknown"
	}
}

// readFrontCause reads the front's detailed proof with the collector's read order and short circuits.
// The ripe sequence and errors are also those of frontProven.
//
// Cost: one count read for a block held above one. A block at one adds the reads of Records.unborn,
// which end at the list read for a listed block.
func readFrontCause(ctx context.Context, rec *Records, blocks block.StoreOps, cursor uint64, key []byte) (FrontCause, uint64, error) {
	// Require exactly a block key, then zero holders, ripe at a sequence the
	// cursor reached.
	if bkey, ok := nextBlockKey(key); !ok || len(bkey) != len(key) {
		return FrontHeldByHolders, 0, errBadBlockKey
	}
	holders, ripe, err := rec.holders(ctx, key)
	if err != nil || holders > 1 {
		return FrontHeldByHolders, 0, err
	}
	if holders == 1 {
		// One holder may be only the implied holder of an unborn block.
		unborn, err := rec.unborn(ctx, blocks, key)
		if err != nil || !unborn {
			return FrontHeldByHolders, 0, err
		}
		return FrontUnborn, 0, nil
	}
	if ripe > cursor {
		return FrontRipening, ripe, nil
	}

	// A listed block needs its queue or parked entry, and one the store
	// does not list needs its bytes gone.
	listed, err := rec.listed(ctx, key)
	if err != nil {
		return FrontHeldByHolders, 0, err
	}
	if listed {
		proven, err := rec.tx.Exists(ctx, rec.queueKey(ripe, key))
		if err == nil && !proven {
			proven, err = rec.tx.Exists(ctx, rec.releasedKey(ripe, key))
		}
		if err == nil && !proven {
			proven, err = rec.tx.Exists(ctx, rec.parkedKey(ripe, key))
		}
		if err != nil || !proven {
			return FrontZeroUnqueued, ripe, err
		}
		return FrontPoppable, ripe, nil
	}
	ref, _, err := ParseBlockKey(key)
	if err != nil {
		return FrontHeldByHolders, 0, err
	}
	stored, err := blocks.GetBlockExists(ctx, ref)
	if err != nil || stored {
		return FrontDamaged, ripe, err
	}
	return FrontPoppable, ripe, nil
}
