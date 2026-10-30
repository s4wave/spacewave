package kvtx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// requireRefsKnown refuses a write that would store a block whose writer did
// not declare its references, so every block the Volume stores has a child
// list and no mark or count meets a block that hides its children. An entry
// without RefsKnown passes only when store already holds its block: the write
// stores nothing, and the block keeps the list of its first write. Called
// with a physical transaction's blocks, the check and the write share the
// Volume's one writer, so no other writer stores or frees the block between
// them. A direct write checks outside the writer, so a collector that frees
// the block between the check and the write leaves it stored without a list:
// a hole the mark stops at, which frees nothing. Returns
// block.ErrRefsUnknown naming the first refused block.
func requireRefsKnown(ctx context.Context, store block.StoreOps, entries []*block.PutBatchEntry) error {
	// Collect the blocks written without their references.
	var unknown []*block.BlockRef
	for _, entry := range entries {
		if entry.Tombstone || entry.RefsKnown || len(entry.Data) == 0 {
			continue
		}
		ref := entry.Ref
		if ref.GetEmpty() {
			var err error
			ref, err = block.BuildBlockRef(entry.Data, &block.PutOpts{HashType: store.GetHashType()})
			if err != nil {
				return err
			}
		}
		unknown = append(unknown, ref)
	}
	if len(unknown) == 0 {
		return nil
	}

	// Refuse the first one the store does not hold.
	exists, err := store.GetBlockExistsBatch(ctx, unknown)
	if err != nil {
		return err
	}
	for i, ok := range exists {
		if !ok {
			return errors.Wrap(block.ErrRefsUnknown, unknown[i].MarshalString())
		}
	}
	return nil
}

// requireRefKnown is requireRefsKnown for one block written by PutBlock.
func requireRefKnown(ctx context.Context, store block.StoreOps, data []byte, opts *block.PutOpts) error {
	// A block whose writer declared its references, or no block, passes.
	if opts.GetRefsKnown() || len(data) == 0 {
		return nil
	}

	// Name the block the way the store will.
	ref := opts.GetForceBlockRef()
	if ref.GetEmpty() {
		var err error
		ref, err = block.BuildBlockRef(data, &block.PutOpts{HashType: opts.SelectHashType(store.GetHashType())})
		if err != nil {
			return err
		}
	}
	return requireRefsKnown(ctx, store, []*block.PutBatchEntry{{Ref: ref, Data: data}})
}
