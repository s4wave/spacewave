package block_gc

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// LandFill stores the entries of a cache fill, which copies blocks a peer
// holds, with the child list of each block it gives birth to and a fill mark
// on it. A fill holds nothing, so it journals no holder and reads no liveness
// state: an optimistic fill conflicts only with a commit that stores or frees
// one of its blocks, never with the journal's bookkeeping. A block it gives
// birth to keeps the holder its record implies, as an orphan put does, and
// the children its list names gain no holder from it, so the collector frees
// a marked block without releasing them. It returns, for each entry, whether
// blocks held its bytes before.
func LandFill(ctx context.Context, rec *Records, blocks block.StoreOps, entries []*block.PutBatchEntry) ([]bool, error) {
	// Name each entry and find the blocks without a child list.
	born := make(map[string]*block.PutBatchEntry)
	for _, entry := range entries {
		if entry == nil || entry.Tombstone {
			continue
		}
		if len(entry.Data) == 0 {
			return nil, block.ErrEmptyBlock
		}
		if !entry.RefsKnown {
			return nil, errors.Wrap(block.ErrRefsUnknown, entry.Ref.MarshalString())
		}
		if entry.Ref.GetEmpty() {
			ref, err := block.BuildBlockRef(entry.Data, &block.PutOpts{HashType: blocks.GetHashType()})
			if err != nil {
				return nil, err
			}
			entry.Ref = ref
		}
		listed, err := rec.HasChildren(ctx, entry.Ref)
		if err != nil {
			return nil, err
		}
		if !listed {
			born[string(BlockKey(entry.Ref))] = entry
		}
	}

	// Store the bytes, then write each born block's child list and mark.
	existed, err := blocks.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	for _, entry := range born {
		if err := rec.PutChildren(ctx, entry.Ref, entry.Refs); err != nil {
			return nil, err
		}
		if err := rec.tx.Set(ctx, rec.key(kindFill, BlockKey(entry.Ref)), nil); err != nil {
			return nil, err
		}
	}
	return existed, nil
}

// takeFillMark deletes the fill mark of ref and reports whether it had one.
func (r *Records) takeFillMark(ctx context.Context, ref *block.BlockRef) (bool, error) {
	key := r.key(kindFill, BlockKey(ref))
	marked, err := r.tx.Exists(ctx, key)
	if err != nil || !marked {
		return false, err
	}
	return true, r.tx.Delete(ctx, key)
}
