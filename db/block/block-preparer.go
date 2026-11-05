package block

import "context"

// AtomicBlockPreparer stores blocks with their bucket ownership in one
// transaction, without moving a root. Calls are synchronous; the caller keeps
// its buffers until return. An empty bucketID disables GC bookkeeping. A store
// that cannot returns ErrAtomicCommitUnsupported before any I/O.
type AtomicBlockPreparer interface {
	// PrepareOwnedBlock stores one block with bucket ownership.
	PrepareOwnedBlock(ctx context.Context, bucketID string, data []byte, opts *PutOpts) (*BlockRef, bool, error)
	// PrepareOwnedBlockBatch stores a batch with bucket ownership. It returns
	// whether each block was stored before the write, like PutBlockBatch.
	PrepareOwnedBlockBatch(ctx context.Context, bucketID string, entries []*PutBatchEntry) ([]bool, error)
}
