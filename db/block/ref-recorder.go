package block

import "context"

// BlockRefRecorder records block reference edges for GC tracking.
// StoreOps implementations that support GC can implement this interface.
// Transaction.Write() checks for this interface and records refs
// automatically after each block is written.
type BlockRefRecorder interface {
	// RecordBlockRefs records that source references targets.
	// Called after PutBlock with the refs extracted from the decoded block.
	RecordBlockRefs(ctx context.Context, source *BlockRef, targets []*BlockRef) error
}
