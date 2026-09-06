package blob

import (
	"context"

	"github.com/aperturerobotics/util/conc"
	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

const (
	// Streaming reads overlap at most eight chunks. The window is four MiB,
	// except that a larger demanded chunk is read alone.
	chunkReadAheadCount   = 8
	chunkReadAheadBytes   = 4 << 20
	chunkReadAheadMinRead = 32 << 10
)

// chunkReadAhead belongs to one Reader. Only the reader accesses pending;
// workers resolve their own promises. Closing cancels and joins every fetch
// before the reader releases its cursors.
type chunkReadAhead struct {
	ctx     context.Context
	cancel  context.CancelFunc
	chunks  *sbset.SubBlockSet
	queue   *conc.ConcurrentQueue
	pending map[int]*promise.Promise[[]byte]
}

func newChunkReadAhead(ctx context.Context, chunks *sbset.SubBlockSet) *chunkReadAhead {
	ctx, cancel := context.WithCancel(ctx)
	return &chunkReadAhead{
		ctx:     ctx,
		cancel:  cancel,
		chunks:  chunks,
		queue:   conc.NewConcurrentQueue(chunkReadAheadCount),
		pending: make(map[int]*promise.Promise[[]byte], chunkReadAheadCount),
	}
}

// read fills the bounded forward window and returns only the requested chunk's
// result. A speculative error remains attached to its chunk until requested.
func (r *chunkReadAhead) read(idx int) ([]byte, error) {
	for previous := range r.pending {
		if previous < idx {
			delete(r.pending, previous)
		}
	}

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

		result := promise.NewPromise[[]byte]()
		r.pending[next] = result
		r.queue.Enqueue(func() {
			data, err := fetchChunkDataNoCursorCache(r.ctx, chunk, cursor, next, nil)
			result.SetResult(data, err)
		})
	}
	return r.pending[idx].Await(r.ctx)
}

func (r *chunkReadAhead) close() {
	r.cancel()
	_ = r.queue.WaitIdle(context.Background(), nil)
	clear(r.pending)
}
