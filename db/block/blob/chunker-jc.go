package blob

import (
	"context"
	"io"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/cdc/jc"
)

// buildChunkIndexJC builds the jc-chunked block index.
// appends if there are already chunks, recording them with chunks
// returns new total size and error
func buildChunkIndexJC(
	ctx context.Context,
	rdr io.Reader,
	bcs *block.Cursor,
	ci *ChunkIndex,
	chunks *chunkAppender,
) (uint64, error) {
	// Configure the chunk index to use the JC chunker.
	chunkerArgs := ci.GetChunkerArgs()
	if chunkerArgs == nil {
		ci.ChunkerArgs = &ChunkerArgs{}
		chunkerArgs = ci.ChunkerArgs
	}
	chunkerArgs.ChunkerType = ChunkerType_ChunkerType_JC

	// Read the JC chunk boundaries and key from the index arguments.
	jcArgs := chunkerArgs.GetJcArgs()
	minChunkSize, targetChunkSize, maxChunkSize := jcArgs.GetChunkingMinSize(), jcArgs.GetChunkingTargetSize(), jcArgs.GetChunkingMaxSize()

	// Apply default JC boundaries for sizes omitted by the caller.
	if minChunkSize == 0 {
		minChunkSize = DefChunkingMinSize
	}
	if targetChunkSize == 0 {
		targetChunkSize = DefChunkingTargetSize
	}
	if maxChunkSize == 0 {
		maxChunkSize = DefChunkingMaxSize
	}

	// Open the JC chunker and release its internal buffers on return.
	chunker, err := jc.NewChunkerWithOptions(
		rdr,
		minChunkSize,
		maxChunkSize,
		targetChunkSize,
		jcArgs.GetKey(),
	)
	if err != nil {
		return 0, err
	}
	defer chunker.Reset() // clear internal buffers

	// Resume chunk positions after the existing chunk index.
	var idx int
	var totalSize uint64
	var chkStart uint64
	if oldChunks := ci.GetChunks(); len(oldChunks) != 0 {
		chk := oldChunks[len(oldChunks)-1]
		chkStart = chk.Start + chk.Size
		totalSize += chkStart
		idx += len(oldChunks)
	}

	// Use a local buffer for chunk data
	chunkBuf := make([]byte, maxChunkSize)

	// Cut the remaining reader bytes into indexed JC chunks.
	for {
		// Read the next JC chunk until the reader reaches its end.
		nchk, err := chunker.Next(chunkBuf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}

		// Append the JC chunk and advance the index byte position.
		totalSize += uint64(nchk.Length)                                                     //nolint:gosec // chunker lengths are nonnegative and bounded by the fixed chunk buffer.
		if err := chunks.append(idx, uint64(nchk.Length), chkStart, nchk.Data); err != nil { //nolint:gosec // chunker lengths are nonnegative and bounded by the fixed chunk buffer.
			return 0, err
		}
		chkStart += uint64(nchk.Length) //nolint:gosec // chunker lengths are nonnegative and bounded by the fixed chunk buffer.
		idx++

		// Stop chunk construction when the request context is canceled.
		if err := ctx.Err(); err != nil {
			return 0, context.Canceled
		}
	}

	// Flush the appended chunks and publish the updated index on the cursor.
	if err := chunks.flush(); err != nil {
		return 0, err
	}
	bcs.SetBlock(ci, true)
	return totalSize, nil
}
