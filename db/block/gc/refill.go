package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// FillDrops counts the copied entries of a cache fill RefillBlocks drops:
// entries whose refs the writer copied rather than read from content, which
// the records cannot take as a child list.
type FillDrops struct {
	// Blocks counts the copied entries of blocks the records do not list.
	Blocks int
	// Holes counts the copied entries of recorded holes, which only a put
	// that read its refs from content refills.
	Holes int
	// At are the dropped entries' indexes in the batch.
	At []int
}

// RefillBlocks stores the bytes of each entry whose block the records list
// and do not hide, such as a placed store's evicted block, and touches no
// record: the list and count stay exact while the bytes are gone. It drops
// each copied entry, one whose refs the writer did not read from content, of
// a block the records do not list, counting it in the FillDrops, since the
// records would hold its copied refs as the block's children; a copied entry
// of a parked block, whose list the records keep, is not dropped. It returns
// the other entries, blocks the records lack, hide or hold as a hole, for
// the caller to Land under a hold, and, for each entry, whether blocks held
// its bytes before; an entry it left or dropped reads false.
//
// It reads each entry's child list in one batch and, until the generation is
// Settled, whether each listed block is hidden, as classifyPuts does, so a
// fill in an optimistic transaction conflicts with the commit that frees or
// hides one of its blocks.
func RefillBlocks(ctx context.Context, rec *Records, blocks block.StoreOps, entries []*block.PutBatchEntry) ([]*block.PutBatchEntry, []bool, FillDrops, error) {
	// Sort the blocks by what the records hold for each.
	kinds := make(map[string]putKind, len(entries))
	err := classifyPuts(ctx, rec, blocks, entries, nil, func(entry *block.PutBatchEntry, kind putKind) error {
		kinds[string(BlockKey(entry.Ref))] = kind
		return nil
	})
	if err != nil {
		return nil, nil, FillDrops{}, err
	}

	// Split the listed blocks from the rest, and drop the copied entries the
	// records would take as lists.
	var refill, rest []*block.PutBatchEntry
	var at []int
	var drops FillDrops
	for i, entry := range entries {
		if entry == nil || entry.Tombstone {
			continue
		}
		kind := kinds[string(BlockKey(entry.Ref))]
		switch {
		case kind == putStored:
			refill, at = append(refill, entry), append(at, i)
		case entry.RefsKnown || kind == putParked:
			rest = append(rest, entry)
		case kind == putRefill:
			drops.Holes, drops.At = drops.Holes+1, append(drops.At, i)
		default:
			drops.Blocks, drops.At = drops.Blocks+1, append(drops.At, i)
		}
	}

	// Store the listed blocks' bytes.
	existed := make([]bool, len(entries))
	if len(refill) == 0 {
		return rest, existed, drops, nil
	}
	stored, err := blocks.PutBlockBatch(ctx, refill)
	if err != nil {
		return nil, nil, FillDrops{}, err
	}
	for j, i := range at {
		existed[i] = j < len(stored) && stored[j]
	}
	return rest, existed, drops, nil
}
