package packmove

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// BlockPacker is the bucket side of a pack: a store that writes each batch of
// blocks as one packfile. block_store_s3.PackStore implements it.
type BlockPacker interface {
	// PutBlockBatch writes the batch as one packfile and its entry. The batch
	// must fit one packfile.
	PutBlockBatch(ctx context.Context, batch []*block.PutBatchEntry) ([]bool, error)
	// GetBlockExistsBatch reports which refs a packfile holds.
	GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error)
}
