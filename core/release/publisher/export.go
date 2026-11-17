package publisher

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/hash"
)

// Export emits the verified release closure as content-addressed CDN packs.
// The caller exclusively owns the World until Export returns, retains emitted
// packs, and publishes the returned head only after every pack is durable.
// Export does not read credentials or contact a remote service.
func Export(ctx context.Context, eng world.Engine, metadata *release.ReleaseMetadata, spaceID string, emit delta.ChunkEmitter) (*bucket.ObjectRef, []*packfile.PackfileEntry, error) {
	// Verify the complete closure before emitting any artifact.
	if eng == nil || spaceID == "" || emit == nil {
		return nil, nil, errors.New("release World, Space ID, and pack emitter are required")
	}
	if err := metadata.Validate(); err != nil {
		return nil, nil, err
	}
	closure, err := collectBlocks(ctx, eng, metadata)
	if err != nil {
		return nil, nil, err
	}

	// Preserve the same manifest-local ordering and pack limits as
	// publication, reading each block's data back as its pack fills.
	var entries []*packfile.PackfileEntry
	err = closure.read(ctx, eng, func(read loadBlock) error {
		index := 0
		var err error
		entries, err = delta.EmitDeltaChunks(ctx, spaceID, func() (*hash.Hash, *block.StoredBlock, error) {
			// Pull the next closure block, reading its data only now.
			if index == len(closure.blocks) {
				return nil, nil, nil
			}
			entry := &closure.blocks[index]
			index++
			stored, err := read(ctx, entry)
			if err != nil {
				return nil, nil, err
			}
			return entry.ref.GetHash(), stored, nil
		}, delta.DefaultMaxChunkBytes, emit)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	head := closure.head
	return head, entries, nil
}
