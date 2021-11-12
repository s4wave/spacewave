package blob

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/restic/chunker"
)

// buildChunkIndexRabin builds the rabin-chunked block index.
// appends if there are already chunks
// returns new total size and error
func buildChunkIndexRabin(
	ctx context.Context,
	rdr io.Reader,
	bcs *block.Cursor,
	ci *ChunkIndex,
) (uint64, error) {
	chunkerArgs := ci.GetChunkerArgs()
	if chunkerArgs == nil {
		ci.ChunkerArgs = &ChunkerArgs{}
	}
	chunkerArgs.ChunkerType = ChunkerType_ChunkerType_RABIN
	rabinArgs := chunkerArgs.GetRabinArgs()
	if rabinArgs == nil {
		rabinArgs = &RabinArgs{}
		chunkerArgs.RabinArgs = rabinArgs
	}

	poly := chunker.Pol(rabinArgs.GetPol())
	if poly == 0 {
		if ciPol := ci.GetChunkerArgs().GetRabinArgs().GetPol(); ciPol != 0 {
			poly = chunker.Pol(ciPol)
		} else {
			var err error
			poly, err = chunker.RandomPolynomial()
			if err != nil {
				return 0, err
			}
			rabinArgs.Pol = uint64(poly)
		}
	}

	chkSet := ci.GetChunkSet(bcs)
	minChunkSize, maxChunkSize := rabinArgs.GetChunkingMinSize(), rabinArgs.GetChunkingMaxSize()
	if minChunkSize == 0 {
		minChunkSize = defChunkingMinSize
	}
	if maxChunkSize == 0 {
		maxChunkSize = defChunkingMaxSize
	}
	if maxChunkSize <= minChunkSize {
		maxChunkSize = minChunkSize + 1
	}

	chk := chunker.NewWithBoundaries(
		rdr,
		poly,
		uint(minChunkSize),
		uint(maxChunkSize),
	)
	var idx int
	var totalSize uint64
	var chkStart uint64
	if oldChunks := ci.GetChunks(); len(oldChunks) != 0 {
		chk := oldChunks[len(oldChunks)-1]
		chkStart = chk.Start + chk.Size
		totalSize += chkStart
		idx += len(oldChunks)
	}
	for {
		// note: we have to allocate 1 buffer per chunk here.
		nchk, err := chk.Next(nil)
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}

		dataSlice := nchk.Data
		totalSize += uint64(nchk.Length)
		ci.AppendChunk(chkSet, idx, uint64(nchk.Length), chkStart, dataSlice)
		chkStart += uint64(nchk.Length)
		idx++
	}
	if len(ci.Chunks) <= 1 {
		rabinArgs.Pol = 0
	}
	bcs.SetBlock(ci, true)
	return totalSize, nil
}
