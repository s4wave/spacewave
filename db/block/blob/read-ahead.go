package blob

import (
	"context"

	"github.com/aperturerobotics/util/conc"
	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

const (
	// chunkReadAheadCount limits streaming reads to eight overlapping
	// chunks. The window is four MiB, except that a larger demanded chunk
	// is read alone.
	chunkReadAheadCount   = 8
	chunkReadAheadBytes   = 4 << 20
	chunkReadAheadMinRead = 32 << 10
)

// chunkReadAhead belongs to one Reader. Only the reader accesses pending.
// Workers resolve their own promises. Closing cancels and joins every fetch
// before the reader releases its cursors.
type chunkReadAhead struct {
	ctx     context.Context
	cancel  context.CancelFunc
	chunks  *sbset.SubBlockSet
	blobEnd uint64
	queue   *conc.ConcurrentQueue
	pending map[int]*promise.Promise[[]byte]
}

func newChunkReadAhead(ctx context.Context, chunks *sbset.SubBlockSet) *chunkReadAhead {
	// The last chunk ends the stored bytes. Start sums the preceding sizes.
	var blobEnd uint64
	if value, _ := chunks.Get(chunks.Len() - 1); value != nil {
		if last, ok := value.(*Chunk); ok {
			blobEnd = last.GetStart() + last.GetSize()
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	return &chunkReadAhead{
		ctx:     ctx,
		cancel:  cancel,
		chunks:  chunks,
		blobEnd: blobEnd,
		queue:   conc.NewConcurrentQueue(chunkReadAheadCount),
		pending: make(map[int]*promise.Promise[[]byte], chunkReadAheadCount),
	}
}

// read fills the bounded forward window and returns only the requested chunk's
// result. A speculative error remains attached to its chunk until requested.
func (r *chunkReadAhead) read(idx int) ([]byte, error) {
	// Drop pending reads behind the requested chunk.
	for previous := range r.pending {
		if previous < idx {
			delete(r.pending, previous)
		}
	}

	// Fill the forward window with chunk fetches.
	var total uint64
	end := min(r.chunks.Len(), idx+chunkReadAheadCount)
	for next := idx; next < end; next++ {
		value, cursor := r.chunks.Get(next)
		chunk, ok := value.(*Chunk)
		if !ok || cursor == nil {
			if next == idx {
				return nil, block.ErrUnexpectedType
			}
			break
		}
		size := chunk.GetSize()
		if next > idx && (size > chunkReadAheadBytes || total > chunkReadAheadBytes-size) {
			break
		}
		total += size
		if r.pending[next] != nil {
			continue
		}

		// The stream reads on past this chunk, and an access-ordered pack
		// stores those chunks after it, so a miss may fetch up to a window of
		// the blob's remaining bytes in one transport request.
		result := promise.NewPromise[[]byte]()
		r.pending[next] = result
		readAhead := min(chunkReadAheadBytes, r.blobEnd-chunk.GetStart())
		fetchCtx := block.WithReadAhead(r.ctx, int(readAhead)) //nolint:gosec // bounded by chunkReadAheadBytes.
		r.queue.Enqueue(func() {
			data, err := fetchChunkDataNoCursorCache(fetchCtx, chunk, cursor, next, nil)
			result.SetResult(data, err)
		})
	}

	// Await the requested chunk's result.
	return r.pending[idx].Await(r.ctx)
}

func (r *chunkReadAhead) close() {
	r.cancel()
	_ = r.queue.WaitIdle(context.Background(), nil)
	clear(r.pending)
}
