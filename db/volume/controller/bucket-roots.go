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
	// SetBucketRoots writes entries a bucket owns and moves its named roots in
	// one transaction.
	SetBucketRoots(context.Context, string, []*block.PutBatchEntry, []block.NamedRoot) error
	// PinBucketRoot protects a reader's root until the returned release.
	PinBucketRoot(context.Context, *block.BlockRef) (func(), error)
	// OpenStage opens a stage owner node, released by the returned func.
	OpenStage(context.Context) (string, func(), error)
	// PrepareStagedBlock writes a block the stage owns.
	PrepareStagedBlock(context.Context, string, []byte, *block.PutOpts) (*block.BlockRef, bool, error)
	// PrepareStagedBlockBatch writes a batch of blocks the stage owns.
	PrepareStagedBlockBatch(context.Context, string, []*block.PutBatchEntry) error
	// ReleaseBucketRoots drops a bucket's staging edges to roots.
	ReleaseBucketRoots(context.Context, string, []*block.BlockRef) error
	// ReleaseStageRoots drops a stage's edges to roots.
	ReleaseStageRoots(context.Context, string, []*block.BlockRef) error
	// MarkRootsComplete records completion proofs for stored roots.
	MarkRootsComplete(context.Context, []*block.BlockRef) error
	// RootComplete reports whether a root has a completion proof.
	RootComplete(context.Context, *block.BlockRef) (bool, error)
}

// MarkRootsComplete records a fenced World in the volume ownership graph.
func (b *bucketHandle) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return b.v.(bucketRootVolume).MarkRootsComplete(ctx, roots)
}

// RootComplete checks the volume's lifetime-bound World proof.
func (b *bucketHandle) RootComplete(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return b.v.(bucketRootVolume).RootComplete(ctx, ref)
}

// SupportsRootRetention requires ownership and bytes in one physical transaction.
func (b *bucketHandle) SupportsRootRetention() bool {
	v, ok := b.v.(bucketRootVolume)
	return ok && b.readOps == nil && v.SupportsAtomicPublication() && b.gcOps != nil && !b.gcOps.HasWALAppender()
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

// _ is a type assertion
var _ block.RootRetainer = (*bucketHandle)(nil)
