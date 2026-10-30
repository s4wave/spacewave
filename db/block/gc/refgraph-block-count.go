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
