package block_store_writeback

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/net/hash"
)

// Mark records a written block.
type Mark struct {
	// Hash is the block hash.
	Hash *hash.Hash
	// Size is the block size in bytes.
	Size int64
	// At is when the block was written. The zero time uploads it without
	// waiting to settle, as a backfill does.
	At time.Time
}

// MarkFunc durably records written blocks before their writes are
// acknowledged. A batch write passes all of its blocks in one call.
type MarkFunc func(ctx context.Context, marks []Mark) error
