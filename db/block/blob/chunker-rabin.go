package blob

import (
	"context"
	"io"

	"github.com/restic/chunker"
	"github.com/s4wave/spacewave/db/block"
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
// appends if there are already chunks, recording them with chunks
// returns new total size and error
func buildChunkIndexRabin(
	ctx context.Context,
	rdr io.Reader,
	bcs *block.Cursor,
	ci *ChunkIndex,
	chunks *chunkAppender,
) (uint64, error) {
	// Configure the chunk index to use the Rabin chunker.
	chunkerArgs := ci.GetChunkerArgs()
	if chunkerArgs == nil {
		ci.ChunkerArgs = &ChunkerArgs{}
		chunkerArgs = ci.ChunkerArgs
	}
	chunkerArgs.ChunkerType = ChunkerType_ChunkerType_RABIN

	// Choose the configured, random, or default Rabin polynomial.
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

	// Apply valid Rabin chunk size boundaries with defaults for omitted sizes.
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

	// Open the Rabin chunker with the selected polynomial and boundaries.
	chk := chunker.New(
		rdr,
		poly,
		chunker.WithBoundaries(uint(minChunkSize), uint(maxChunkSize)),
	)

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

	// Cut the remaining reader bytes into indexed Rabin chunks.
	for {
		// note: we have to allocate 1 buffer per chunk here.
		nchk, err := chk.Next(nil)
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}

		// Append the Rabin chunk and advance the index byte position.
		totalSize += uint64(nchk.Length)
		if err := chunks.append(idx, uint64(nchk.Length), chkStart, nchk.Data); err != nil {
			return 0, err
		}
		chkStart += uint64(nchk.Length)
		idx++
	}

	// Flush pending chunks and publish the completed Rabin index.
	if err := chunks.flush(); err != nil {
		return 0, err
	}
	if len(ci.Chunks) <= 1 && rabinArgs.Pol == uint64(defRabinPol) {
		rabinArgs.Pol = 0
	}
	bcs.SetBlock(ci, true)
	return totalSize, nil
}
