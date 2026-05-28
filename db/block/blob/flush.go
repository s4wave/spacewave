package blob

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// appendChunkData records a chunk and stores its data with bounded heap use.
// When the chunk index is attached to a transaction with a backing store, the
// data block is written directly and only its DataRef is retained in the
// in-memory ChunkIndex. Ephemeral cursors fall back to the cursor graph path.
func appendChunkData(
	ctx context.Context,
	ci *ChunkIndex,
	chkSet *sbset.SubBlockSet,
	idx int,
	size, start uint64,
	data []byte,
) error {
	if ref, ok, err := putChunkDataDirect(ctx, chkSet, data); ok || err != nil {
		if err != nil {
			return err
		}
		ci.Chunks = append(ci.Chunks, &Chunk{
			DataRef: ref,
			Size:    size,
			Start:   start,
		})
		return nil
	}

	dataCopy := append([]byte(nil), data...)
	ci.AppendChunk(chkSet, idx, size, start, dataCopy)
	return flushChunkData(ctx, chkSet, idx)
}

func putChunkDataDirect(
	ctx context.Context,
	chkSet *sbset.SubBlockSet,
	data []byte,
) (*block.BlockRef, bool, error) {
	if chkSet == nil || chkSet.GetCursor() == nil {
		return nil, false, nil
	}
	tx := chkSet.GetCursor().GetTransaction()
	if tx == nil || tx.GetStoreOps() == nil {
		return nil, false, nil
	}

	writeData := data
	if xfrm := tx.GetTransformer(); xfrm != nil {
		var err error
		writeData, err = xfrm.EncodeBlock(writeData)
		if err != nil {
			return nil, true, err
		}
	}
	opts := tx.GetPutOpts().CloneVT()
	opts.ForceBlockRef = nil
	opts.Refs = nil
	ref, _, err := tx.GetStoreOps().PutBlock(ctx, writeData, opts)
	return ref, true, err
}

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
