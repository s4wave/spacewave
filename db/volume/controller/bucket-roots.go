package volume_controller

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// bucketRootVolume is a volume that owns named roots, reader pins and stages
// in its block ownership graph.
type bucketRootVolume interface {
	// SupportsAtomicPublication reports whether bytes and ownership share one
	// physical transaction. The other methods require it.
	SupportsAtomicPublication() bool
	// KeepsHoldCounts reports whether the volume keeps shared counts.
	KeepsHoldCounts() bool
	// SetBucketRoots writes entries a bucket owns and moves its named roots in
	// one transaction.
	SetBucketRoots(context.Context, string, []*block.PutBatchEntry, []block.NamedRoot) error
	// PinBucketRoot protects a reader's root until the returned release.
	PinBucketRoot(context.Context, *block.BlockRef) (func(), error)
	// OpenStage opens a stage owner node, released by the returned func.
	OpenStage(context.Context) (string, func(), error)
	// PrepareStagedBlock writes a block the stage owns.
	PrepareStagedBlock(context.Context, string, []byte, *block.PutOpts) (*block.BlockRef, bool, error)
	// PrepareStagedBlockBatch writes a batch of blocks the stage owns and reports
	// which of them were stored before.
	PrepareStagedBlockBatch(context.Context, string, []*block.PutBatchEntry) ([]bool, error)
	// ReleaseBucketRoots drops a bucket's staging edges to roots.
	ReleaseBucketRoots(context.Context, string, []*block.BlockRef) error
	// ReleaseStageRoots drops a stage's edges to roots.
	ReleaseStageRoots(context.Context, string, []*block.BlockRef) error
	// HoldStageCompleteRoots reports which roots have a completion proof and
	// makes the stage own each of them.
	HoldStageCompleteRoots(context.Context, string, []*block.BlockRef) ([]bool, error)
	// MarkRootsComplete records completion proofs for stored roots.
	MarkRootsComplete(context.Context, []*block.BlockRef) error
	// RootsComplete reports which roots have a completion proof, reading all
	// of them in one transaction.
	RootsComplete(context.Context, []*block.BlockRef) ([]bool, error)
}

// MarkRootsComplete records a fenced World in the volume ownership graph.
func (b *bucketHandle) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return b.v.(bucketRootVolume).MarkRootsComplete(ctx, roots)
}

// RootsComplete checks the volume's lifetime-bound World proofs.
func (b *bucketHandle) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return b.v.(bucketRootVolume).RootsComplete(ctx, refs)
}

// SupportsRootRetention requires ownership and bytes in one physical transaction.
func (b *bucketHandle) SupportsRootRetention() bool {
	v, ok := b.v.(bucketRootVolume)
	return ok && b.readOps == nil && v.SupportsAtomicPublication() && b.gcOps != nil
}

// KeepsHoldCounts reports whether the volume keeps shared counts.
func (b *bucketHandle) KeepsHoldCounts() bool {
	return b.SupportsRootRetention() && b.v.(bucketRootVolume).KeepsHoldCounts()
}

// SetRetainedRoots writes entries owned by this bucket and replaces its named
// durable roots in one volume transaction.
func (b *bucketHandle) SetRetainedRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if !b.SupportsRootRetention() {
		return nil
	}
	return b.v.(bucketRootVolume).SetBucketRoots(ctx, b.t.bucketID, entries, roots)
}

// PinRoot retains a reader's root until the returned release is called.
func (b *bucketHandle) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	if !b.SupportsRootRetention() {
		return func() {}, nil
	}
	return b.v.(bucketRootVolume).PinBucketRoot(ctx, ref)
}

// OpenStage returns a handle whose writes a new volume stage owns.
func (b *bucketHandle) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	// A volume without root retention writes through the bucket.
	if !b.SupportsRootRetention() {
		return b, func() {}, nil
	}

	// Open the stage and clone the handle around it.
	stage, release, err := b.v.(bucketRootVolume).OpenStage(ctx)
	if err != nil {
		return nil, nil, err
	}
	staged := b.clone()
	staged.stage = stage
	return staged, release, nil
}

// ReleaseRoots drops the staging ownership of roots held by the owner of this
// handle's writes: its stage on a staged handle, otherwise its bucket.
func (b *bucketHandle) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	if !b.SupportsRootRetention() {
		return nil
	}
	if b.stage != "" {
		return b.v.(bucketRootVolume).ReleaseStageRoots(ctx, b.stage, refs)
	}
	return b.v.(bucketRootVolume).ReleaseBucketRoots(ctx, b.t.bucketID, refs)
}

// HoldCompleteRoots reports which roots have a completion proof and makes this
// handle's stage own each of them in one volume transaction. A handle without a
// stage reports no proofs.
func (b *bucketHandle) HoldCompleteRoots(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	if b.stage == "" || !b.SupportsRootRetention() {
		return make([]bool, len(refs)), nil
	}
	return b.v.(bucketRootVolume).HoldStageCompleteRoots(ctx, b.stage, refs)
}

// _ is a type assertion
var (
	_ block.RootRetainer       = (*bucketHandle)(nil)
	_ block.CompleteRootHolder = (*bucketHandle)(nil)
)
