package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
)

// BlockCount is a block's count record.
type BlockCount struct {
	// Count is the number of the block's holders.
	Count uint64
	// Queued reports a free queue entry, which a block has at zero.
	Queued bool
	// Holds reports that the block still holds its children.
	Holds bool
}

// GetBlockCount reads a block's count record and whether it exists.
func (g *RefGraph) GetBlockCount(ctx context.Context, node string) (BlockCount, bool, error) {
	var count BlockCount
	var found bool
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		// Read the record, if any.
		rec, ok, err := readCountTx(ctx, tx, node)
		if err != nil || !ok {
			return err
		}

		// Report it.
		found = true
		count = BlockCount{Count: rec.count, Queued: rec.count == 0, Holds: rec.count != 0 || rec.holds}
		return nil
	})
	return count, found, err
}

// CountHolds reports whether the counts say something holds block: they are
// complete or being built, and its count is above zero. A count being built
// counts only holders the build reached, each of which exists, so a count
// above zero means a holder either way. Counts in doubt or never started say
// nothing.
func (g *RefGraph) CountHolds(ctx context.Context, block string) (bool, error) {
	var held bool
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		// Read the counts' state; only kept counts say anything.
		c, err := openCounter(ctx, tx)
		if err != nil || !c.on {
			return err
		}

		// Read the block's count.
		rec, found, err := readCountTx(ctx, tx, block)
		held = found && rec.count != 0
		return err
	})
	return held, err
}
