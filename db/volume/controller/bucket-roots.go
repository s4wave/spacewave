package volume_controller

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

type bucketRootVolume interface {
	SupportsAtomicPublication() bool
	SetBucketRoot(context.Context, string, string, *block.BlockRef) error
	PinBucketRoot(context.Context, *block.BlockRef) (func(), error)
	MarkRootsComplete(context.Context, []*block.BlockRef) error
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

// SetRetainedRoot replaces one named durable root for this bucket.
func (b *bucketHandle) SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error {
	if !b.SupportsRootRetention() {
		return nil
	}
	return b.v.(bucketRootVolume).SetBucketRoot(ctx, b.t.bucketID, name, ref)
}

// PinRoot retains a reader's root until the returned release is called.
func (b *bucketHandle) PinRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	if !b.SupportsRootRetention() {
		return func() {}, nil
	}
	return b.v.(bucketRootVolume).PinBucketRoot(ctx, ref)
}
