package blob

import (
	"context"

	"github.com/aperturerobotics/hydra/block/sbset"
)

// flushChunkData flushes the data block at chunk index idx to storage.
// This writes the ByteSlice to the block store immediately, freeing the
// in-memory data. The block's ref is kept so the parent transaction's
// Write() skips re-encoding it.
// No-op if the transaction has no backing store.
func flushChunkData(ctx context.Context, chkSet *sbset.SubBlockSet, idx int) error {
	_, chkBcs := chkSet.Get(idx)
	if chkBcs == nil {
		return nil
	}
	dataBcs := chkBcs.GetExistingRef(1)
	if dataBcs == nil {
		return nil
	}
	tx := dataBcs.GetTransaction()
	if tx == nil || tx.GetStoreOps() == nil {
		return nil
	}
	_, _, err := tx.WriteAtRoot(ctx, true, dataBcs)
	return err
}
