package block

import "context"

// PutBatchEntry describes one durable block write in a lower-layer batch.
type PutBatchEntry struct {
	// Ref is the expected content-addressed block reference.
	Ref *BlockRef
	// Data is the encoded block payload.
	Data []byte
	// Tombstone marks the block ref as deleted.
	Tombstone bool
}

// BatchPutStore is implemented by stores that can durably publish blocks in a batch.
type BatchPutStore interface {
	// PutBlockBatch durably writes the supplied block operations as one
	// lower-layer batch.
	PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) error
}

// BatchExistsStore is implemented by stores that can probe block existence in
// a batch.
type BatchExistsStore interface {
	// GetBlockExistsBatch checks whether each block reference exists.
	GetBlockExistsBatch(ctx context.Context, refs []*BlockRef) ([]bool, error)
}

// BackgroundPutStore is implemented by stores that support low-priority
// background block writes. Background writes are deprioritized relative
// to foreground writes, useful for GC operations and other non-latency-
// sensitive work.
type BackgroundPutStore interface {
	// PutBlockBackground writes a single block at background priority.
	PutBlockBackground(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error)
}
