package volume

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// Roots is the part of a Volume that holds what keeps blocks live: the named
// roots of its buckets, reader pins, stages and completion proofs. It is part
// of Volume, so a wrapper forwards it rather than erasing it.
//
// A volume without root retention, whose store cannot commit liveness with
// the blocks, reports none: it holds no named roots, refuses a root that
// would hold a block as block.SetRootsUnheld does, pins and stages nothing,
// and proves nothing. Every block written to it stays stored.
type Roots interface {
	// SupportsRootRetention reports whether the volume holds roots, pins and
	// stages.
	SupportsRootRetention() bool
	// GetBucketRoot returns the block a bucket's root name holds, or nil.
	GetBucketRoot(ctx context.Context, bucketID, name string) (*block.BlockRef, error)
	// ListBucketRoots calls fn with each root of a bucket whose name starts
	// with prefix.
	ListBucketRoots(ctx context.Context, bucketID, prefix string, fn func(block.NamedRoot) error) error
	// SetBucketRoots writes entries a bucket owns and moves its named roots in
	// one transaction, refusing the change with block.ErrRootMoved when a root
	// does not hold its Previous.
	SetBucketRoots(ctx context.Context, bucketID string, entries []*block.PutBatchEntry, roots []block.NamedRoot) error
	// PinBucketRoot holds a reader's root until the returned release.
	PinBucketRoot(ctx context.Context, ref *block.BlockRef) (func(), error)
	// OpenStage opens a stage, released by the returned func, and returns its
	// name.
	OpenStage(ctx context.Context) (string, func(), error)
	// PrepareStagedBlock writes a block the stage holds.
	PrepareStagedBlock(ctx context.Context, stage string, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error)
	// PrepareStagedBlockBatch writes a batch of blocks the stage holds and
	// reports which of them were stored before.
	PrepareStagedBlockBatch(ctx context.Context, stage string, entries []*block.PutBatchEntry) ([]bool, error)
	// ReleaseStageRoots drops the stage's holds on refs.
	ReleaseStageRoots(ctx context.Context, stage string, refs []*block.BlockRef) error
	// HoldStageCompleteRoots reports which roots have a completion proof and
	// makes the stage hold each of them.
	HoldStageCompleteRoots(ctx context.Context, stage string, refs []*block.BlockRef) ([]bool, error)
	// MarkRootsComplete records completion proofs for stored roots.
	MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error
	// RootsComplete reports which roots have a completion proof, reading all
	// of them in one transaction.
	RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error)
}
