package blob

import (
	"context"
	"io"
	"math"
	"runtime/trace"
	"sort"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// BuildChunkIndex constructs a chunk index.
// Blocks will be written to the block transaction.
// If bcs already contains a ChunkIndex, it will be reused.
// If poly is zero, the default constant polynomial will be used.
func BuildChunkIndex(
	ctx context.Context,
	rdr io.Reader,
	bcs *block.Cursor,
	chunkerArgs *ChunkerArgs,
) (*ChunkIndex, uint64, error) {
	ci, totalSize, _, err := buildChunkIndex(ctx, rdr, bcs, chunkerArgs, false)
	return ci, totalSize, err
}

// buildChunkIndex chunks rdr onto the chunk index at bcs after its existing
// chunks. With holdTail it does not store the last chunk, which ends at the end
// of rdr rather than at a boundary, and returns it so the caller can keep its
// bytes as tail chunks. Otherwise the last chunk starts the tail.
func buildChunkIndex(
	ctx context.Context,
	rdr io.Reader,
	bcs *block.Cursor,
	chunkerArgs *ChunkerArgs,
	holdTail bool,
) (*ChunkIndex, uint64, *heldChunk, error) {
	// Reuse the chunk index at the cursor and merge the chunker arguments.
	ci, err := UnmarshalChunkIndex(ctx, bcs)
	if err != nil {
		if err != block.ErrUnexpectedType {
			return nil, 0, nil, err
		}
	}
	if ci == nil {
		ci = &ChunkIndex{}
	}
	if ci.ChunkerArgs == nil {
		ci.ChunkerArgs = &ChunkerArgs{}
	}
	ci.ChunkerArgs.ApplyArgs(chunkerArgs)

	// Cut and store the chunks with the configured chunker.
	chunks := newChunkAppender(ctx, ci, ci.GetChunkSet(bcs))
	chunks.holdLast = holdTail
	chunkerType := ci.ChunkerArgs.GetChunkerType()
	var totalSize uint64
	switch chunkerType {
	case ChunkerType_ChunkerType_JC, ChunkerType_ChunkerType_DEFAULT:
		totalSize, err = buildChunkIndexJC(ctx, rdr, bcs, ci, chunks)
	case ChunkerType_ChunkerType_RABIN:
		totalSize, err = buildChunkIndexRabin(ctx, rdr, bcs, ci, chunks)
	default:
		err = errors.Wrap(ErrUnknownChunkerType, chunkerType.String())
	}
	if err != nil {
		return nil, 0, nil, err
	}

	// Start the tail at the held chunk or at the last stored chunk.
	held := chunks.held
	switch n := len(ci.Chunks); {
	case held != nil:
		ci.TailStart = held.start
	case n != 0:
		ci.TailStart = ci.Chunks[n-1].GetStart()
	default:
		ci.TailStart = 0
	}
	return ci, totalSize, held, nil
}

// AppendChunk appends a chunk with the given data.
func (r *ChunkIndex) AppendChunk(chkSet *sbset.SubBlockSet, idx int, size, start uint64, data []byte) {
	r.Chunks = append(r.Chunks, &Chunk{
		Size:  size,
		Start: start,
	})
	_, chkBcs := chkSet.Get(idx)
	dataBcs := chkBcs.FollowRef(1, nil)
	dataBcs.SetBlock(byteslice.NewByteSlice(&data), true)
}

// ReadFromChunks reads up to len(buf) data from the chunks, starting at byte
// index start. It checks the chunk at chunkIdx first and otherwise searches for
// the chunk containing start. Save outChunkIdx and pass it again when stepping
// through the chunks sequentially. Returns io.EOF if start is past the last
// chunk.
func ReadFromChunks(
	ctx context.Context,
	chunkSet *sbset.SubBlockSet,
	buf []byte,
	start, chunkIdx int,
) (n int, outChunkIdx int, err error) {
	return readFromChunks(ctx, chunkSet, buf, start, chunkIdx, nil)
}

// readFromChunks implements ReadFromChunks with an optional chunk cache.
func readFromChunks(
	ctx context.Context,
	chunkSet *sbset.SubBlockSet,
	buf []byte,
	start, chunkIdx int,
	cache *chunkReadCache,
) (n int, outChunkIdx int, err error) {
	// Locate and validate the chunk containing the requested byte offset.
	chunkIdx, err = findChunk(chunkSet, start, chunkIdx)
	if err != nil {
		return 0, 0, err
	}
	chunkBlk, chunkCursor := chunkSet.Get(chunkIdx)
	if chunkCursor == nil {
		// A chunk set without a cursor has no fetchable data.
		return 0, 0, io.EOF
	}
	chunk, ok := chunkBlk.(*Chunk)
	if !ok {
		return 0, 0, block.ErrUnexpectedType
	}

	// findChunk checked that the chunk bounds fit in an int.
	readStartPos := start - int(chunk.GetStart())                  //nolint:gosec
	readEndPos := min(readStartPos+len(buf), int(chunk.GetSize())) //nolint:gosec
	data, err := fetchChunkDataNoCursorCache(ctx, chunk, chunkCursor, chunkIdx, cache)
	if err != nil {
		return 0, 0, err
	}
	n = copy(buf, data[readStartPos:readEndPos])
	return n, chunkIdx, nil
}

// findChunk returns the index of the chunk containing byte start. It checks
// hint first so sequential reads skip the search, then binary searches the
// ordered chunks without following their cursors. Returns io.EOF if start is
// past the last chunk.
func findChunk(chunkSet *sbset.SubBlockSet, start, hint int) (int, error) {
	// Compute chunk ends while retaining any invalid type or bounds error.
	var boundsErr error
	chunkEnd := func(idx int) int {
		// Require chunk metadata before computing its byte range.
		chunk, ok := chunkSet.GetSubBlock(idx).(*Chunk)
		if !ok {
			boundsErr = block.ErrUnexpectedType
			return math.MaxInt
		}

		// Validate that the chunk byte range fits in an integer.
		chunkStart, chunkSize := chunk.GetStart(), chunk.GetSize()
		if chunkStart > math.MaxInt || chunkSize > math.MaxInt-chunkStart {
			boundsErr = errors.New("chunk bounds exceed maximum")
			return math.MaxInt
		}
		return int(chunkStart + chunkSize) //nolint:gosec
	}

	// Reuse the chunk hint or search the ordered index for the requested offset.
	chunkLen := chunkSet.Len()
	idx := hint
	if idx < 0 || idx >= chunkLen || chunkEnd(idx) <= start || (idx > 0 && chunkEnd(idx-1) > start) {
		idx = sort.Search(chunkLen, func(i int) bool {
			return chunkEnd(i) > start
		})
	}
	if boundsErr != nil {
		return 0, boundsErr
	}
	if idx == chunkLen {
		return 0, io.EOF
	}

	// Chunks are contiguous, so only a corrupt index leaves start in a gap.
	chunk := chunkSet.GetSubBlock(idx).(*Chunk)
	if int(chunk.GetStart()) > start { //nolint:gosec
		return 0, errors.New("chunk index does not cover offset")
	}
	return idx, nil
}

// fetchChunkDataNoCursorCache returns chunk data without retaining it in the
// cursor. A non-nil cache serves repeated reads and retains the fetched data.
// The caller has checked that the chunk size fits in an int.
func fetchChunkDataNoCursorCache(
	ctx context.Context,
	chunk *Chunk,
	chunkCursor *block.Cursor,
	chunkIdx int,
	cache *chunkReadCache,
) ([]byte, error) {
	// Reuse cached chunk bytes and reserve capacity for a cache miss.
	if cache != nil {
		if data, ok := cache.get(chunkIdx); ok {
			return data, nil
		}
		cache.evict(int(chunk.GetSize())) //nolint:gosec
	}

	// Fetch chunk bytes through read-ahead or the chunk cursor.
	var data []byte
	var err error
	if cache != nil && cache.ahead != nil {
		data, err = cache.ahead.read(chunkIdx)
	} else if trace.IsEnabled() {
		_, task := trace.NewTask(ctx, "db/block/blob/chunk-fetch")
		data, err = chunk.FetchDataNoCache(ctx, chunkCursor, false)
		task.End()
	} else {
		data, err = chunk.FetchDataNoCache(ctx, chunkCursor, false)
	}
	if err != nil {
		return nil, err
	}

	// Retain fetched chunk bytes for subsequent cached reads.
	if cache != nil {
		cache.add(chunkIdx, data)
	}
	return data, nil
}
