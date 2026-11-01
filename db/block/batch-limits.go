package block

import "time"

// BatchLimits describes the batches a store takes best from PutBlockBatch.
type BatchLimits struct {
	// Bytes is the block bytes of a full batch.
	Bytes int64
	// Blocks is the block count of a full batch.
	Blocks int
	// Workers is how many batches may be in flight at once.
	Workers int
	// Linger is how long a short batch of settled blocks waits for more
	// blocks before it is written anyway. Zero writes it at once.
	Linger time.Duration
}

// BatchSizer is implemented by a store that keeps each batch as one unit, such
// as a packfile, so an uploader fills the batch before it writes.
type BatchSizer interface {
	// GetBatchLimits returns the batch the store takes best.
	GetBatchLimits() BatchLimits
}
