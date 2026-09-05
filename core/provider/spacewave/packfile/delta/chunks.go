package delta

import (
	"bytes"
	"context"
	"encoding/binary"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/identity"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

// DefaultMaxChunkBytes is the encoded byte ceiling for a delta chunk.
// The default leaves headroom under the Worker sync/push body cap.
const DefaultMaxChunkBytes int64 = writer.DefaultMaxPackBytes

// ChunkEmitter receives complete KVFiles in emission order. Returning an error
// stops emission immediately; previously emitted chunks remain the caller's
// responsibility.
type ChunkEmitter func(ctx context.Context, idx int, entry *packfile.PackfileEntry, data []byte) error

// EmitDeltaChunks packs iterator blocks in order, bounded by maxBytes including
// the KVFile index and footer. A non-positive limit uses DefaultMaxChunkBytes.
// A single block that cannot fit returns an error without emitting that block.
// On success, entries correspond to callbacks in emission order. Errors return
// no entries, even if earlier callbacks succeeded.
func EmitDeltaChunks(
	ctx context.Context,
	resourceID string,
	iter writer.BlockIterator,
	maxBytes int64,
	emit ChunkEmitter,
) ([]*packfile.PackfileEntry, error) {
	// Validate the stream and resolve the packing limits.
	if iter == nil {
		return nil, errors.New("iter is nil")
	}
	if emit == nil {
		return nil, errors.New("emit callback is nil")
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxChunkBytes
	}
	maxBlocks := int(writer.DefaultPolicy().MaxBlocksPerPack)

	// Carry one lookahead block between chunks without advancing past it.
	var emitted []*packfile.PackfileEntry
	chunkIdx := 0

	// pendingBlock carries one lookahead block from one chunk to the next so
	// the byte-ceiling check can decide to close before packing it.
	var pendingHash *hash.Hash
	var pendingData []byte

	// Encode and publish each complete chunk before consuming the next.
	for {
		var chunkBuf bytes.Buffer
		var chunkBytes int64
		indexBytes := int64(8) // KVFile trailing index-entry count.
		var sizeBuf [binary.MaxVarintLen64]byte
		var chunkBlocks int
		chunkClosed := false

		chunkIter := func() (*hash.Hash, []byte, error) {
			if chunkClosed {
				return nil, nil, nil
			}

			// Consume the lookahead before advancing the source iterator.
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			h, data, err := pendingHash, pendingData, error(nil)
			pendingHash, pendingData = nil, nil
			if h == nil {
				h, data, err = iter()
				if err != nil {
					return nil, nil, err
				}
				if h == nil {
					return nil, nil, nil
				}
			}

			// Include the generated entry, its size varint, and its fixed-width
			// index position before accepting the block into this chunk.
			entry := kvfile.IndexEntry{
				Key:    []byte(h.MarshalString()),
				Offset: uint64(chunkBytes),
				Size:   uint64(len(data)),
			}
			entrySize := entry.SizeVT()
			entryBytes := int64(entrySize + binary.PutUvarint(sizeBuf[:], uint64(entrySize)) + 8)
			remaining := maxBytes - chunkBytes - indexBytes - entryBytes
			if int64(len(data)) > remaining || (maxBlocks > 0 && chunkBlocks >= maxBlocks) {
				if chunkBlocks == 0 {
					return nil, nil, errors.Errorf("block %s cannot fit in a %d-byte encoded pack", h.MarshalString(), maxBytes)
				}
				pendingHash, pendingData = h, data
				chunkClosed = true
				return nil, nil, nil
			}
			chunkBytes += int64(len(data))
			indexBytes += entryBytes
			chunkBlocks++
			return h, data, nil
		}

		// Finalize the KVFile and derive its content-addressed identity.
		res, err := writer.PackBlocks(&chunkBuf, chunkIter)
		if err != nil {
			return nil, errors.Wrap(err, "pack delta chunk")
		}
		if res.BlockCount == 0 {
			break
		}
		packID, err := identity.BuildPackID(resourceID, res)
		if err != nil {
			return nil, errors.Wrap(err, "build delta pack id")
		}

		// Transfer the complete pack to the caller before advancing.
		entry := &packfile.PackfileEntry{
			Id:                 packID,
			BloomFilter:        res.BloomFilter,
			BloomFormatVersion: packfile.BloomFormatVersionV1,
			BlockCount:         res.BlockCount,
			SizeBytes:          res.BytesWritten,
			CreatedAt:          timestamppb.New(time.Now().UTC()),
		}
		if err := emit(ctx, chunkIdx, entry, chunkBuf.Bytes()); err != nil {
			return nil, err
		}
		emitted = append(emitted, entry)
		chunkIdx++

		if pendingHash == nil {
			break
		}
	}

	return emitted, nil
}
