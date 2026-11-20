package blob

import (
	"context"
	"io"
	"runtime"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// AddressChunks cuts rdr into chunks and addresses each one: it encodes the
// chunk with xfrm and hashes the result with hashType, the ref a transaction
// with that transformer would store it under. It passes each chunk to fn in
// order and stores nothing. The returned index equals the one BuildBlob writes
// for the same bytes and chunker arguments, so a caller can write the Blob and
// keep the chunk bytes elsewhere, such as in the file they came from.
func AddressChunks(
	ctx context.Context,
	rdr io.Reader,
	chunkerArgs *ChunkerArgs,
	xfrm block.Transformer,
	hashType hash.HashType,
	fn func(*Chunk) error,
) (*ChunkIndex, uint64, error) {
	// Run the appender with a sink in place of a transaction.
	if hashType == 0 {
		hashType = block.DefaultHashType
	}
	ci := &ChunkIndex{ChunkerArgs: &ChunkerArgs{}}
	ci.ChunkerArgs.ApplyArgs(chunkerArgs)
	a := &chunkAppender{
		ctx:      ctx,
		ci:       ci,
		window:   min(runtime.GOMAXPROCS(0), maxChunkEncodeWindow),
		sink:     fn,
		xfrm:     xfrm,
		hashType: hashType,
	}

	// Cut with the configured chunker.
	var totalSize uint64
	var err error
	switch t := ci.ChunkerArgs.GetChunkerType(); t {
	case ChunkerType_ChunkerType_JC, ChunkerType_ChunkerType_DEFAULT:
		totalSize, err = buildChunkIndexJC(ctx, rdr, nil, ci, a)
	case ChunkerType_ChunkerType_RABIN:
		totalSize, err = buildChunkIndexRabin(ctx, rdr, nil, ci, a)
	default:
		err = errors.Wrap(ErrUnknownChunkerType, t.String())
	}
	if err != nil {
		return nil, 0, err
	}
	if n := len(ci.Chunks); n != 0 {
		ci.TailStart = ci.Chunks[n-1].GetStart()
	}
	return ci, totalSize, nil
}
