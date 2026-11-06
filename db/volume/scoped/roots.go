package volume_scoped

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// HoldsRoots reports whether the underlying volume holds roots,
// pins and stages.
func (v *Volume) HoldsRoots() bool {
	return v.inner.HoldsRoots()
}

// GetRoot returns the block the view's own root name holds, or nil. The
// view's own roots are the roots of its bucket with the empty ID, which the
// prefix keeps apart from every other owner's.
func (v *Volume) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return v.GetBucketRoot(ctx, "", name)
}

// ListRoots lists the view's own roots whose name starts with prefix.
func (v *Volume) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return v.ListBucketRoots(ctx, "", prefix, fn)
}

// SetRoots stores entries and moves the view's own roots in one transaction.
func (v *Volume) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return v.SetBucketRoots(ctx, "", entries, roots)
}

// GetBucketRoot returns the block a root of the view's bucket holds, or nil.
func (v *Volume) GetBucketRoot(ctx context.Context, bucketID, name string) (*block.BlockRef, error) {
	return v.inner.GetBucketRoot(ctx, v.bucketStore.prefix+bucketID, name)
}

// ListBucketRoots lists the roots of the view's bucket whose name starts with
// prefix.
func (v *Volume) ListBucketRoots(ctx context.Context, bucketID, prefix string, fn func(block.NamedRoot) error) error {
	return v.inner.ListBucketRoots(ctx, v.bucketStore.prefix+bucketID, prefix, fn)
}

// SetBucketRoots stores entries and moves the roots of the view's bucket in
// one transaction of the underlying volume. A tombstone is refused, as
// PutBlockBatch refuses it.
func (v *Volume) SetBucketRoots(ctx context.Context, bucketID string, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if err := refuseTombstones(entries); err != nil {
		return err
	}
	return v.inner.SetBucketRoots(ctx, v.bucketStore.prefix+bucketID, entries, roots)
}

// PinBucketRoot holds a reader's root on the underlying volume. A block hash
// is the capability to read the block, so a pin passes through.
func (v *Volume) PinBucketRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	return v.inner.PinBucketRoot(ctx, ref)
}

// OpenStage opens a stage on the underlying volume and names it under the
// view's prefix, so the view's stage calls reach only the stages it opened.
func (v *Volume) OpenStage(ctx context.Context) (string, func(), error) {
	// Open the stage.
	stage, release, err := v.inner.OpenStage(ctx)
	if err != nil {
		return "", nil, err
	}

	// Record it under the view's name until its release.
	name := v.bucketStore.prefix + stage
	v.stageMtx.Lock()
	v.stages[name] = struct{}{}
	v.stageMtx.Unlock()
	return name, func() {
		v.stageMtx.Lock()
		delete(v.stages, name)
		v.stageMtx.Unlock()
		release()
	}, nil
}

// PrepareStagedBlock writes a block the view's stage holds.
func (v *Volume) PrepareStagedBlock(ctx context.Context, stage string, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	inner, err := v.innerStage(stage)
	if err != nil {
		return nil, false, err
	}
	return v.inner.PrepareStagedBlock(ctx, inner, data, opts)
}

// PrepareStagedBlockBatch writes a batch the view's stage holds.
func (v *Volume) PrepareStagedBlockBatch(ctx context.Context, stage string, entries []*block.PutBatchEntry) ([]bool, error) {
	inner, err := v.innerStage(stage)
	if err == nil {
		err = refuseTombstones(entries)
	}
	if err != nil {
		return nil, err
	}
	return v.inner.PrepareStagedBlockBatch(ctx, inner, entries)
}

// ReleaseStageRoots drops the view's stage's holds on refs.
func (v *Volume) ReleaseStageRoots(ctx context.Context, stage string, refs []*block.BlockRef) error {
	inner, err := v.innerStage(stage)
	if err != nil {
		return err
	}
	return v.inner.ReleaseStageRoots(ctx, inner, refs)
}

// HoldStageCompleteRoots reports which roots have a completion proof and
// makes the view's stage hold each of them.
func (v *Volume) HoldStageCompleteRoots(ctx context.Context, stage string, refs []*block.BlockRef) ([]bool, error) {
	inner, err := v.innerStage(stage)
	if err != nil {
		return nil, err
	}
	return v.inner.HoldStageCompleteRoots(ctx, inner, refs)
}

// MarkRootsComplete records completion proofs on the underlying volume.
func (v *Volume) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	return v.inner.MarkRootsComplete(ctx, roots)
}

// RootsComplete reports which roots have a completion proof.
func (v *Volume) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return v.inner.RootsComplete(ctx, refs)
}

// innerStage returns the underlying volume's name of a stage this view opened,
// or refuses a stage it did not.
func (v *Volume) innerStage(stage string) (string, error) {
	// Look the stage up among the view's open ones.
	v.stageMtx.Lock()
	_, open := v.stages[stage]
	v.stageMtx.Unlock()
	if !open {
		return "", errors.Wrap(ErrRefused, "stage not opened by this view")
	}
	return strings.TrimPrefix(stage, v.bucketStore.prefix), nil
}

// refuseTombstones refuses a batch that deletes a block, which the volume's
// collector owns.
func refuseTombstones(entries []*block.PutBatchEntry) error {
	for _, entry := range entries {
		if entry.Tombstone {
			return errors.Wrap(ErrRefused, "tombstone block")
		}
	}
	return nil
}
