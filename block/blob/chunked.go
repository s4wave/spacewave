package blob

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/aperturerobotics/hydra/block/sbset"
	"github.com/pkg/errors"
	"github.com/restic/chunker"
)

// BuildChunkIndex constructs a chunk index.
// Blocks will be written to the block transaction.
// The new root Blob block will become the root of bcs.
// If poly is zero, a random polynomial will be selected.
func BuildChunkIndex(
	ctx context.Context,
	rdr io.Reader,
	bcs *block.Cursor,
	poly chunker.Pol,
	minChunkSize, maxChunkSize uint64,
) (*ChunkIndex, uint64, error) {
	var err error
	if poly == 0 {
		poly, err = chunker.RandomPolynomial()
		if err != nil {
			return nil, 0, err
		}
	}

	ci := &ChunkIndex{Pol: uint64(poly)}
	bcs.SetBlock(ci, true)
	chkSet := ci.GetChunkSet(bcs)

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
	for {
		// note: we have to allocate 1 buffer per chunk here.
		nchk, err := chk.Next(nil)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, 0, err
		}

		dataSlice := nchk.Data
		totalSize += uint64(nchk.Length)
		ci.Chunks = append(ci.Chunks, &Chunk{
			Size:  uint64(nchk.Length),
			Start: uint64(nchk.Start),
		})
		_, chkBcs := chkSet.Get(idx)
		dataBcs := chkBcs.FollowRef(1, nil)
		dataBcs.SetBlock(byteslice.NewByteSlice(&dataSlice), true)
		idx++
	}
	if len(ci.Chunks) <= 1 {
		ci.Pol = 0
	}
	return ci, totalSize, nil
}

// ReadFromChunks reads up to len(buf) data from the chunks, starting at byte index start.
// Attempts to start reading from chunkIdx, but will search for the chunk containing start.
// If the chunk at chunkIdx does not contain start, will binary-search for the appropriate chunk.
// The value of outChunkIdx should be saved and passed again when stepping through the chunks sequentially.
// Returns io.EOF if start is past the last chunk.
func ReadFromChunks(
	chunkSet *sbset.SubBlockSet,
	buf []byte,
	start, chunkIdx int,
) (n int, outChunkIdx int, err error) {
	chunkLen := chunkSet.Len()
	if chunkIdx >= chunkLen {
		chunkIdx = chunkLen - 1
	}

	// NOTE: this could be faster with a binary search.
	for {
		// check if chunkIdx is within range
		currChunkBlk, currChunkBcs := chunkSet.Get(chunkIdx)
		if currChunkBcs == nil {
			err = io.EOF
			return n, outChunkIdx, err
		}
		currChunk, ok := currChunkBlk.(*Chunk)
		if !ok {
			return n, outChunkIdx, block.ErrUnexpectedType
		}
		currChunkStart, currChunkSize := currChunk.GetStart(), currChunk.GetSize()
		currChunkEnd := currChunkStart + currChunkSize
		if int(currChunkStart) > start {
			chunkIdx--
			continue
		}
		if int(currChunkEnd) <= start {
			chunkIdx++
			continue
		}
		readStartPos := start - int(currChunkStart)
		readEndPos := readStartPos + len(buf)
		if readEndPos > int(currChunkSize) {
			readEndPos = int(currChunkSize)
		}

		var data []byte
		var dataOk bool
		currChunkDataCs := currChunk.FollowDataRef(currChunkBcs)
		currChunkBlki, _ := currChunkDataCs.GetBlock()
		if currChunkBlki != nil {
			currChunkBlk, ok := currChunkBlki.(*byteslice.ByteSlice)
			if ok {
				data = currChunkBlk.GetBytes()
				dataOk = len(data) != 0
			}
		}
		if !dataOk {
			data, dataOk, err = currChunkDataCs.Fetch()
			if err != nil {
				return n, outChunkIdx, err
			}
		}
		if !dataOk {
			return n, outChunkIdx, errors.Errorf(
				"chunk data block not found: <%q>",
				currChunkDataCs.GetRef().MarshalString(),
			)
		}
		if len(data) != int(currChunkSize) {
			return n, outChunkIdx, errors.Errorf(
				"expected chunk %s data len %d but got %d",
				currChunkDataCs.GetRef().MarshalString(),
				int(currChunkSize),
				len(data),
			)
		}
		copy(buf, data[readStartPos:readEndPos])
		n = readEndPos - readStartPos
		outChunkIdx = chunkIdx
		return n, outChunkIdx, nil
	}
}
