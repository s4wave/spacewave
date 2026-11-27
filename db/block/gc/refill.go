package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// FillFate describes what RefillBlocks does with one cache fill entry.
type FillFate uint8

const (
	// FillSkip ignores a nil or tombstone entry.
	FillSkip FillFate = iota
	// FillStoreBytes stores the bytes of a visible listed block.
	FillStoreBytes
	// FillLand passes a parked block or an entry with known refs to Land under a hold.
	FillLand
	// FillDropHole drops a copied entry of a recorded hole.
	FillDropHole
	// FillDropBlock drops other copied entries, including listed blocks hidden while Sweeping.
	FillDropBlock
)

// FillDrops counts the copied entries of a cache fill RefillBlocks drops:
// entries whose refs the writer copied rather than read from content, which
// the records cannot take as a child list.
type FillDrops struct {
	// Blocks counts copied entries dropped for the reasons FillDropBlock describes.
	Blocks int
	// Holes counts the copied entries of recorded holes, which only a put
	// that read its refs from content refills.
	Holes int
	// At are the dropped entries' indexes in the batch.
	At []int
}

// FillFates classifies cache fills in input order from one view of the records.
// Like Land, it names entries with empty refs from their bytes. It writes no
// storage records or block bytes.
func FillFates(ctx context.Context, rec *Records, blocks block.StoreOps, entries []*block.PutBatchEntry) ([]FillFate, error) {
	// Read the records once per block, with the visibility checks shared by Land.
	kinds := make(map[string]putKind, len(entries))
	err := classifyPuts(ctx, rec, blocks, entries, nil, func(entry *block.PutBatchEntry, kind putKind) error {
		kinds[string(BlockKey(entry.Ref))] = kind
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Apply the records' classification to each entry's own refs, including duplicates.
	fates := make([]FillFate, len(entries))
	for i, entry := range entries {
		if entry == nil || entry.Tombstone {
			continue
		}
		kind := kinds[string(BlockKey(entry.Ref))]
		switch {
		case kind == putStored:
			fates[i] = FillStoreBytes
		case entry.RefsKnown || kind == putParked:
			fates[i] = FillLand
		case kind == putRefill:
			fates[i] = FillDropHole
		default:
			fates[i] = FillDropBlock
		}
	}
	return fates, nil
}

// RefillBlocks stores the bytes of each entry whose block the records list
// and do not hide, such as a placed store's evicted block, and touches no
// record: the list and count stay exact while the bytes are gone. It drops
// each copied entry, one whose refs the writer did not read from content, of
// a recorded hole or a block without a visible list, except for parked
// blocks whose lists Land keeps. It counts the drops and returns the entries
// the caller still needs to Land under a hold. For each input entry it also
// reports whether it refilled bytes the store already held.
//
// It reads each entry's child list in one batch and, until the generation is
// Settled, whether each listed block is hidden, as classifyPuts does, so a
// fill in an optimistic transaction conflicts with the commit that frees or
// hides one of its blocks.
func RefillBlocks(ctx context.Context, rec *Records, blocks block.StoreOps, entries []*block.PutBatchEntry) ([]*block.PutBatchEntry, []bool, FillDrops, error) {
	// Classify the fills before writing any bytes.
	fates, err := FillFates(ctx, rec, blocks, entries)
	if err != nil {
		return nil, nil, FillDrops{}, err
	}

	// Keep the indexes for existence results and for the entries that stored nothing.
	var refill, rest []*block.PutBatchEntry
	var at []int
	var drops FillDrops
	for i, fate := range fates {
		switch fate {
		case FillStoreBytes:
			refill, at = append(refill, entries[i]), append(at, i)
		case FillLand:
			rest = append(rest, entries[i])
		case FillDropHole:
			drops.Holes, drops.At = drops.Holes+1, append(drops.At, i)
		case FillDropBlock:
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
