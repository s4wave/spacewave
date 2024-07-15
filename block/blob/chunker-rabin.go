package blob

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/restic/chunker"
)

// defRabinPol is the default rabin polynomial
//
// To have deterministic chunking we need to use the same polynomial every time.
// Previous versions of the code randomized the polynomial every time. But it's
// better to have deterministic writes than randomize some aspect of the
// chunking every time you write. Randomizing leads to having completely
// different chunks even if we write the same file twice.
//
// There are three options: use constant size chunks, or use a global constant
// rabin polynomial, or set the polynomial in the options and use the same when
// encoding in the future. The default is now to use this constant.
const defRabinPol = chunker.Pol(16983672372569473)

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
		chunkerArgs = ci.ChunkerArgs
	}
	chunkerArgs.ChunkerType = ChunkerType_ChunkerType_RABIN

	var poly chunker.Pol
	rabinArgs := chunkerArgs.GetRabinArgs()
	if ciPol := rabinArgs.GetPol(); ciPol != 0 {
		poly = chunker.Pol(ciPol)
	} else if rabinArgs.GetRandomPol() {
		var err error
		poly, err = chunker.RandomPolynomial()
		if err != nil {
			return 0, err
		}
	} else {
		poly = defRabinPol
	}

	chkSet := ci.GetChunkSet(bcs)
	minChunkSize, maxChunkSize := rabinArgs.GetChunkingMinSize(), rabinArgs.GetChunkingMaxSize()
	if minChunkSize == 0 {
		minChunkSize = DefChunkingMinSize
	}
	if maxChunkSize == 0 {
		maxChunkSize = DefChunkingMaxSize
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
