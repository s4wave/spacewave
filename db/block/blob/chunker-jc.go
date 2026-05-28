package blob

import (
	"context"
	"io"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/cdc/jc"
)

// buildChunkIndexJC builds the jc-chunked block index.
// appends if there are already chunks
// returns new total size and error
func buildChunkIndexJC(
	ctx context.Context,
	rdr io.Reader,
	bcs *block.Cursor,
	ci *ChunkIndex,
) (uint64, error) {
	chunkerArgs := ci.GetChunkerArgs()
	if chunkerArgs == nil {
		ci.ChunkerArgs = &ChunkerArgs{}
		chunkerArgs = ci.ChunkerArgs
	}
	chunkerArgs.ChunkerType = ChunkerType_ChunkerType_JC

	chkSet := ci.GetChunkSet(bcs)
	jcArgs := chunkerArgs.GetJcArgs()
	minChunkSize, targetChunkSize, maxChunkSize := jcArgs.GetChunkingMinSize(), jcArgs.GetChunkingTargetSize(), jcArgs.GetChunkingMaxSize()

	if minChunkSize == 0 {
		minChunkSize = DefChunkingMinSize
	}
	if targetChunkSize == 0 {
		targetChunkSize = DefChunkingTargetSize
	}
	if maxChunkSize == 0 {
		maxChunkSize = DefChunkingMaxSize
	}

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

	for {
		nchk, err := chunker.Next(chunkBuf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}

		totalSize += uint64(nchk.Length) //nolint:gosec
		if err := appendChunkData(ctx, ci, chkSet, idx, uint64(nchk.Length), chkStart, nchk.Data); err != nil {
			return 0, err
		}
		chkStart += uint64(nchk.Length) //nolint:gosec
		idx++

		if err := ctx.Err(); err != nil {
			return 0, context.Canceled
		}
	}

	bcs.SetBlock(ci, true)
	return totalSize, nil
}
