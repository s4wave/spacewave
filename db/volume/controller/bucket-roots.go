package volume_controller

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/tx"
)

// MarkRootsComplete records a fenced World in the volume ownership graph.
func (b *bucketHandle) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return b.v.MarkRootsComplete(ctx, roots)
}

// RootsComplete checks the volume's lifetime-bound World proofs.
func (b *bucketHandle) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return b.v.RootsComplete(ctx, refs)
}

// SupportsRootRetention reports whether the volume holds roots, pins and
// stages for a writable handle.
func (b *bucketHandle) SupportsRootRetention() bool {
	return b.readOps == nil && b.v.SupportsRootRetention()
}

// GetRoot returns the block the bucket's root name holds, or nil. A bucket
// whose volume holds no roots reports none.
func (b *bucketHandle) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return b.v.GetBucketRoot(ctx, b.t.bucketID, name)
}

// ListRoots lists the bucket's roots whose name starts with prefix.
func (b *bucketHandle) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return b.v.ListBucketRoots(ctx, b.t.bucketID, prefix, fn)
}

// SetRoots writes entries owned by this bucket and moves its named roots in one
// volume transaction. A bucket whose volume holds no roots stores the entries
// and refuses a root that would hold a block, as the volume does.
func (b *bucketHandle) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if b.readOps != nil {
		return tx.ErrNotWrite
	}
	return b.v.SetBucketRoots(ctx, b.t.bucketID, entries, roots)
}

// PinRoot retains a reader's root until the returned release is called.
func (b *bucketHandle) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	if !b.SupportsRootRetention() {
		return func() {}, nil
	}
	return b.v.PinBucketRoot(ctx, ref)
}

// OpenStage returns a handle whose writes a new volume stage owns.
func (b *bucketHandle) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	// A volume without root retention writes through the bucket.
	if !b.SupportsRootRetention() {
		return b, func() {}, nil
	}

	// Open the stage and clone the handle around it.
	stage, release, err := b.v.OpenStage(ctx)
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
	return b.v.ReleaseStageRoots(ctx, b.stage, refs)
}

// HoldCompleteRoots reports which roots have a completion proof and makes this
// handle's stage own each of them in one volume transaction. A handle without a
// stage reports no proofs.
func (b *bucketHandle) HoldCompleteRoots(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	if b.stage == "" || !b.SupportsRootRetention() {
		return make([]bool, len(refs)), nil
	}
	return b.v.HoldStageCompleteRoots(ctx, b.stage, refs)
}

// _ is a type assertion
var (
	_ block.RootRetainer       = (*bucketHandle)(nil)
	_ block.CompleteRootHolder = (*bucketHandle)(nil)
)
