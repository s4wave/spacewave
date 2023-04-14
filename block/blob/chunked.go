package blob

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/aperturerobotics/hydra/block/sbset"
	"github.com/pkg/errors"
)

// BuildChunkIndex constructs a chunk index.
// Blocks will be written to the block transaction.
// If bcs already contains a ChunkIndex,
// If poly is zero, a random polynomial will be selected.
func BuildChunkIndex(
	ctx context.Context,
	rdr io.Reader,
	bcs *block.Cursor,
	chunkerArgs *ChunkerArgs,
) (*ChunkIndex, uint64, error) {
	// TODO: support other chunk types
	chunkerType := chunkerArgs.GetChunkerType()
	switch chunkerType {
	case ChunkerType_ChunkerType_DEFAULT:
	case ChunkerType_ChunkerType_RABIN:
	default:
		return nil, 0, errors.Wrap(ErrUnknownChunkerType, chunkerType.String())
	}

	ci, err := UnmarshalChunkIndex(bcs)
	if err != nil {
		if err != block.ErrUnexpectedType {
			return nil, 0, err
		}
	}
	if ci == nil {
		ci = &ChunkIndex{}
	}
	if ci.ChunkerArgs == nil {
		ci.ChunkerArgs = &ChunkerArgs{}
	}
	ci.ChunkerArgs.ApplyArgs(chunkerArgs)

	totalSize, err := buildChunkIndexRabin(ctx, rdr, bcs, ci)
	if err != nil {
		return nil, 0, err
	}
	return ci, totalSize, err
}

// AppendChunk appends a chunk with the given data.
func (i *ChunkIndex) AppendChunk(chkSet *sbset.SubBlockSet, idx int, size, start uint64, data []byte) {
	i.Chunks = append(i.Chunks, &Chunk{
		Size:  uint64(size),
		Start: uint64(start),
	})
	_, chkBcs := chkSet.Get(idx)
	dataBcs := chkBcs.FollowRef(1, nil)
	dataBcs.SetBlock(byteslice.NewByteSlice(&data), true)
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
		// note: start always >= currChunkStart
		readStartPos := start - int(currChunkStart)
		readEndPos := readStartPos + len(buf)
		if readEndPos > int(currChunkSize) {
			readEndPos = int(currChunkSize)
		}

		data, err := currChunk.FetchData(currChunkBcs, false)
		if err != nil {
			return n, outChunkIdx, err
		}
		copy(buf, data[readStartPos:readEndPos])
		n = readEndPos - readStartPos
		outChunkIdx = chunkIdx
		return n, outChunkIdx, nil
	}
}
