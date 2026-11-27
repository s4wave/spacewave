package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// HolderRow is the stored structure of one block or a named root holding it, without its bytes.
type HolderRow struct {
	// Root identifies a named root holding Ref, with other structural fields unset.
	Root *FrontRoot
	// Ref is the block inspected.
	Ref *block.BlockRef
	// Holders is the netted count, including the implied holder of a missing count record.
	Holders uint64
	// Ripe is the sequence stored with a zero count, or zero for a held block.
	Ripe uint64
	// Listed reports that a child list record exists.
	Listed bool
	// Marked reports that the list was written from content.
	Marked bool
	// Stored reports that raw block bytes exist, including bytes hidden by collection.
	Stored bool
	// Queued reports a queue entry at Ripe.
	Queued bool
	// Released reports a released entry at Ripe.
	Released bool
	// Parked reports a parked entry at Ripe.
	Parked bool
	// Front reports that the block has a front entry.
	Front bool
	// Cause is the front proof's cause, evaluated even for blocks outside the front for summaries.
	Cause FrontCause
}

// readHolderRow reads structural records and raw existence from the same snapshot.
func readHolderRow(ctx context.Context, rec *Records, blocks block.StoreOps, cursor uint64, ref *block.BlockRef) (HolderRow, error) {
	// Read the count and the list's content-verification mark.
	row := HolderRow{Ref: ref}
	key := BlockKey(ref)
	var err error
	row.Holders, row.Ripe, err = rec.holders(ctx, key)
	if err != nil {
		return row, err
	}
	if row.Listed, row.Marked, err = rec.listState(ctx, key); err != nil {
		return row, err
	}

	// Read the three proof records at exactly this count's ripe sequence,
	// and the front key.
	if row.Queued, err = rec.hasQueueKey(ctx, row.Ripe, key); err != nil {
		return row, err
	}
	if row.Released, err = rec.hasReleasedKey(ctx, row.Ripe, key); err != nil {
		return row, err
	}
	if row.Parked, err = rec.hasParkedKey(ctx, row.Ripe, key); err != nil {
		return row, err
	}
	if row.Front, err = rec.hasFront(ctx, key); err != nil {
		return row, err
	}

	// Read raw existence and use the collector's proof for the diagnostic cause.
	if row.Stored, err = blocks.GetBlockExists(ctx, ref); err != nil {
		return row, err
	}
	row.Cause, _, err = readFrontCause(ctx, rec, blocks, cursor, key)
	return row, err
}
