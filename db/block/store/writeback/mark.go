package block_store_writeback

import (
	"context"

	"github.com/s4wave/spacewave/net/hash"
)

// Mark names a written block.
type Mark struct {
	// Hash is the block hash.
	Hash *hash.Hash
	// Size is the block size in bytes.
	Size int64
}

// MarkFunc durably records written blocks before their writes are
// acknowledged. A batch write passes all of its blocks in one call.
type MarkFunc func(ctx context.Context, marks []Mark) error
