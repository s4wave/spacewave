package blob

import (
	"bytes"
	"context"
	"runtime"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// maxChunkEncodeWindow bounds the chunks a chunkAppender encodes at once, and
// with them the chunk copies it holds.
const maxChunkEncodeWindow = 8

// chunkAppender records chunks on a chunk index in chunk order with bounded
// heap use. When the index is attached to a transaction with a backing store,
// each data block is staged on the transaction and only its DataRef is
// retained in the in-memory ChunkIndex. Block transforms such as gzip dominate
// the cost of a large import, so the appender encodes up to its window of
// chunks concurrently and stages them in order. Ephemeral cursors fall back to
// the cursor graph path.
type chunkAppender struct {
	ctx    context.Context
	ci     *ChunkIndex
	chkSet *sbset.SubBlockSet
	// tx is the transaction staging the chunk blocks, or nil for the cursor
	// graph path.
	tx *block.Transaction
	// window is the number of chunks encoded at once.
	window int
	// pending lists the chunks being encoded, oldest first.
	pending []*pendingChunk
}

// pendingChunk is a chunk whose data block is being encoded.
type pendingChunk struct {
	idx         int
	size, start uint64
	// data is the encoded block once done is closed.
	data []byte
	err  error
	done chan struct{}
}

// newChunkAppender constructs a chunkAppender for the chunk set.
func newChunkAppender(ctx context.Context, ci *ChunkIndex, chkSet *sbset.SubBlockSet) *chunkAppender {
	a := &chunkAppender{ctx: ctx, ci: ci, chkSet: chkSet, window: 1}
	if chkSet == nil || chkSet.GetCursor() == nil {
		return a
	}
	if tx := chkSet.GetCursor().GetTransaction(); tx != nil && tx.GetStoreOps() != nil {
		a.tx = tx
		a.window = min(runtime.GOMAXPROCS(0), maxChunkEncodeWindow)
	}
	return a
}

// append records the chunk at idx. The appender does not retain data after
// append returns. Call flush after the last chunk.
func (a *chunkAppender) append(idx int, size, start uint64, data []byte) error {
	if a.tx == nil {
		dataCopy := append([]byte(nil), data...)
		recordMetric(a.ctx, Metric{
			Stage:      "chunk-fallback-copy",
			ChunkBytes: len(dataCopy),
			ChunkIndex: idx,
		})
		a.ci.AppendChunk(a.chkSet, idx, size, start, dataCopy)
		return flushChunkData(a.ctx, a.chkSet, idx)
	}

	chk := &pendingChunk{idx: idx, size: size, start: start}
	if a.window == 1 {
		chk.data, chk.err = a.encode(data)
		return a.put(chk)
	}
	chk.done = make(chan struct{})
	dataCopy := bytes.Clone(data)
	go func() {
		chk.data, chk.err = a.encode(dataCopy)
		close(chk.done)
	}()
	a.pending = append(a.pending, chk)
	if len(a.pending) < a.window {
		return nil
	}
	return a.putOldest()
}

// flush stages the chunks still being encoded.
func (a *chunkAppender) flush() error {
	for len(a.pending) != 0 {
		if err := a.putOldest(); err != nil {
			return err
		}
	}
	return nil
}

// encode applies the transaction's block transform to data.
func (a *chunkAppender) encode(data []byte) ([]byte, error) {
	if xfrm := a.tx.GetTransformer(); xfrm != nil {
		return xfrm.EncodeBlock(data)
	}
	return data, nil
}

// putOldest waits for the oldest pending chunk and stages it.
func (a *chunkAppender) putOldest() error {
	chk := a.pending[0]
	a.pending = a.pending[1:]
	<-chk.done
	return a.put(chk)
}

// put stages one encoded chunk on the transaction staging store. The store
// drains in bounded batches, one durable commit each, and the transaction
// write drains the rest before the root that references them.
func (a *chunkAppender) put(chk *pendingChunk) error {
	// Fail with the chunk's encode error.
	if chk.err != nil {
		return chk.err
	}

	// Stage the chunk as a content block and count it as written.
	opts := a.tx.GetPutOpts().CloneVT()
	opts.ForceBlockRef = nil
	opts.Refs = nil
	staged := a.tx.StageWrites(a.ctx, a.tx.GetStoreOps())
	block.RecordWrite(a.ctx, len(chk.data))
	ref, _, err := staged.PutBlock(a.ctx, chk.data, opts)
	if err != nil {
		return err
	}

	// Record the chunk in the metrics and the chunk index.
	recordMetric(a.ctx, Metric{
		Stage:      "chunk-direct-put",
		ChunkBytes: int(chk.size), //nolint:gosec // chunk sizes are bounded by the chunker buffer.
		ChunkIndex: chk.idx,
		DirectPut:  true,
	})
	a.ci.Chunks = append(a.ci.Chunks, &Chunk{
		DataRef: ref,
		Size:    chk.size,
		Start:   chk.start,
	})
	return nil
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
