package volume_controller

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/tx"
)

// bucketRootVolume is a volume that keeps named roots, reader pins and stages
// in its liveness records.
type bucketRootVolume interface {
	// SupportsAtomicPublication reports whether bytes and ownership share one
	// physical transaction. The other methods require it.
	SupportsAtomicPublication() bool
	// GetBucketRoot returns the block a bucket's root name holds, or nil.
	GetBucketRoot(context.Context, string, string) (*block.BlockRef, error)
	// ListBucketRoots calls the func with each root of a bucket whose name
	// starts with the prefix.
	ListBucketRoots(context.Context, string, string, func(block.NamedRoot) error) error
	// SetBucketRoots writes entries a bucket owns and moves its named roots in
	// one transaction, refusing the change with block.ErrRootMoved when a root
	// does not hold its Previous.
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
	return ok && b.readOps == nil && v.SupportsAtomicPublication()
}

// GetRoot returns the block the bucket's root name holds, or nil. A bucket
// whose volume holds no roots reports none.
func (b *bucketHandle) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	if !b.SupportsRootRetention() {
		return nil, nil
	}
	return b.v.(bucketRootVolume).GetBucketRoot(ctx, b.t.bucketID, name)
}

// ListRoots lists the bucket's roots whose name starts with prefix.
func (b *bucketHandle) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	if !b.SupportsRootRetention() {
		return nil
	}
	return b.v.(bucketRootVolume).ListBucketRoots(ctx, b.t.bucketID, prefix, fn)
}

// SetRoots writes entries owned by this bucket and moves its named roots in one
// volume transaction. A bucket whose volume holds no roots stores the entries
// and drops the roots.
func (b *bucketHandle) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if b.readOps != nil {
		return tx.ErrNotWrite
	}
	if !b.SupportsRootRetention() {
		return block.SetRootsUnheld(ctx, b, entries, roots)
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

// ReleaseRoots releases the stage's roots on refs on a staged handle. A
// bucket's own writes hold nothing, so it releases nothing.
func (b *bucketHandle) ReleaseRoots(ctx context.Context, refs []*block.BlockRef) error {
	if !b.SupportsRootRetention() || b.stage == "" {
		return nil
	}
	return b.v.(bucketRootVolume).ReleaseStageRoots(ctx, b.stage, refs)
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
