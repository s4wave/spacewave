package delta

import (
	"bytes"
	"context"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/hash"

	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/identity"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
)

// DefaultMaxChunkBytes is the default byte ceiling a single delta chunk may
// occupy before =EmitDeltaChunks= rolls the iterator into a new chunk. 63 MiB
// leaves headroom under the Worker =sync/push= 64 MiB body cap.
const DefaultMaxChunkBytes int64 = writer.DefaultMaxPackBytes

// ChunkEmitter receives one kvfile chunk at a time, in emission order. =idx=
// counts up from zero. =entry= is the =PackfileEntry= proto constructed from
// the chunk, with =id=, =bloom_filter=, =block_count=, =size_bytes=, and
// =created_at= populated. =data= is the raw kvfile bytes.
//
// The callback may persist, upload, or drop the bytes. Returning an error
// aborts =EmitDeltaChunks= immediately (matches the =OQ-25= "abort without
// posting root" semantics in scope).
type ChunkEmitter func(ctx context.Context, idx int, entry *packfile.PackfileEntry, data []byte) error

// EmitDeltaChunks drives =writer.PackBlocks= one chunk at a time, reading from
// =iter= until it is exhausted. A chunk is closed as soon as appending the next
// block would push the running byte total past =maxBytes=; if =maxBytes= is
// non-positive, =DefaultMaxChunkBytes= is used. Each closed chunk is emitted
// via =emit= with a v1 pack id and =created_at=timestamppb.Now()=.
//
// Returns the emitted =PackfileEntry= list in chunk order (same order as
// =emit= was called).
func EmitDeltaChunks(
	ctx context.Context,
	resourceID string,
	iter writer.BlockIterator,
	maxBytes int64,
	emit ChunkEmitter,
) ([]*packfile.PackfileEntry, error) {
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

	var emitted []*packfile.PackfileEntry
	chunkIdx := 0

	// pendingBlock carries one lookahead block from one chunk to the next so
	// the byte-ceiling check can decide to close before packing it.
	var pendingHash *hash.Hash
	var pendingData []byte

	for {
		var chunkBuf bytes.Buffer
		var chunkBytes int64
		var chunkBlocks int
		chunkClosed := false

		chunkIter := func() (*hash.Hash, []byte, error) {
			if chunkClosed {
				return nil, nil, nil
			}

			h, data, err := pendingHash, pendingData, error(nil)
			if h != nil {
				pendingHash, pendingData = nil, nil
			} else {
				h, data, err = iter()
				if err != nil {
					return nil, nil, err
				}
				if h == nil {
					return nil, nil, nil
				}
			}

			if chunkBytes > 0 && chunkBytes+int64(len(data)) > maxBytes {
				pendingHash, pendingData = h, data
				chunkClosed = true
				return nil, nil, nil
			}
			if maxBlocks > 0 && chunkBlocks >= maxBlocks {
				pendingHash, pendingData = h, data
				chunkClosed = true
				return nil, nil, nil
			}

			chunkBytes += int64(len(data))
			chunkBlocks++
			return h, data, nil
		}

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
