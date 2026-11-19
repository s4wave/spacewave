package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// RefillBlocks stores the bytes of each entry whose block the records list
// and do not hide, such as a placed store's evicted block, and touches no
// record: the list and count stay exact while the bytes are gone. It returns
// the other entries, blocks the records lack, hide or hold as a hole, for
// the caller to Land under a hold, and, for each entry, whether blocks held
// its bytes before; an entry it left reads false.
//
// It reads each entry's child list in one batch and, until the generation is
// Settled, whether each listed block is hidden, as classifyPuts does, so a
// fill in an optimistic transaction conflicts with the commit that frees or
// hides one of its blocks.
func RefillBlocks(ctx context.Context, rec *Records, blocks block.StoreOps, entries []*block.PutBatchEntry) ([]*block.PutBatchEntry, []bool, error) {
	// Sort the blocks by what the records hold for each.
	listed := make(map[string]bool, len(entries))
	err := classifyPuts(ctx, rec, blocks, entries, nil, func(entry *block.PutBatchEntry, kind putKind) {
		listed[string(BlockKey(entry.Ref))] = kind == putStored
	})
	if err != nil {
		return nil, nil, err
	}

	// Split the listed blocks from the rest.
	var refill, rest []*block.PutBatchEntry
	var at []int
	for i, entry := range entries {
		switch {
		case entry == nil || entry.Tombstone:
		case listed[string(BlockKey(entry.Ref))]:
			refill, at = append(refill, entry), append(at, i)
		default:
			rest = append(rest, entry)
		}
	}

	// Store the listed blocks' bytes.
	existed := make([]bool, len(entries))
	if len(refill) == 0 {
		return rest, existed, nil
	}
	stored, err := blocks.PutBlockBatch(ctx, refill)
	if err != nil {
		return nil, nil, err
	}
	for j, i := range at {
		existed[i] = j < len(stored) && stored[j]
	}
	return rest, existed, nil
}
