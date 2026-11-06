package volume_rpc_client

import (
	"context"
	"errors"
	"io"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
)

// rootBatchBytes bounds the block bytes one SetBucketRoots request carries.
// Larger entries are put first in batches of their own.
const rootBatchBytes = 4 << 20

// GetRoot returns the block the volume's own root name holds, or nil.
func (v *ProxyVolume) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return v.GetBucketRoot(ctx, "", name)
}

// ListRoots lists the volume's own roots whose name starts with prefix.
func (v *ProxyVolume) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return v.ListBucketRoots(ctx, "", prefix, fn)
}

// SetRoots stores entries and moves the volume's own roots in one
// transaction of the remote volume.
func (v *ProxyVolume) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return v.SetBucketRoots(ctx, "", entries, roots)
}

// GetBucketRoot returns the block a named root of a bucket holds, or nil.
func (v *ProxyVolume) GetBucketRoot(ctx context.Context, bucketID, name string) (*block.BlockRef, error) {
	resp, err := v.client.GetBucketRoot(ctx, &volume_rpc.GetBucketRootRequest{BucketId: bucketID, Name: name})
	if err != nil {
		return nil, err
	}
	if ref := resp.GetRef(); !ref.GetEmpty() {
		return ref, nil
	}
	return nil, nil
}

// ListBucketRoots calls fn with each root of a bucket whose name starts with
// prefix, as the remote volume streams them.
func (v *ProxyVolume) ListBucketRoots(ctx context.Context, bucketID, prefix string, fn func(block.NamedRoot) error) error {
	// Open the stream.
	strm, err := v.client.ListBucketRoots(ctx, &volume_rpc.ListBucketRootsRequest{BucketId: bucketID, Prefix: prefix})
	if err != nil {
		return err
	}
	defer strm.Close()

	// Pass each root to fn until the stream ends.
	for {
		resp, err := strm.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(resp.GetRoot().ToNamedRoot()); err != nil {
			return err
		}
	}
}

// SetBucketRoots stores entries and moves the roots of a bucket in one
// transaction of the remote volume. Entries beyond rootBatchBytes are put
// first, as a multi-batch writer's, and the remote collector's grace keeps
// them until the move that holds them lands.
func (v *ProxyVolume) SetBucketRoots(ctx context.Context, bucketID string, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	// Put the entries that do not fit with the move, children first.
	tail, size := len(entries), 0
	for tail > 0 && size+len(entries[tail-1].Data) <= rootBatchBytes {
		tail--
		size += len(entries[tail].Data)
	}
	if tail != 0 {
		if _, err := v.PutBlockBatch(ctx, entries[:tail]); err != nil {
			return err
		}
	}

	// Move the roots with the rest.
	req := &volume_rpc.SetBucketRootsRequest{BucketId: bucketID, Entries: volume_rpc.NewPutBatchEntries(entries[tail:])}
	for _, root := range roots {
		req.Roots = append(req.Roots, volume_rpc.NewNamedRoot(root))
	}
	resp, err := v.client.SetBucketRoots(ctx, req)
	if err != nil {
		return err
	}

	// Restore the refusals the server reported.
	switch resp.GetResult() {
	case volume_rpc.SetBucketRootsResult_SET_BUCKET_ROOTS_RESULT_ROOT_MOVED:
		return block.RootMoved(resp.GetName())
	case volume_rpc.SetBucketRootsResult_SET_BUCKET_ROOTS_RESULT_UNHELD:
		return errors.Join(block.ErrRootsUnheld, errors.New(resp.GetName()))
	}
	return nil
}

// HoldsRoots reports whether the remote volume holds roots, pins and stages,
// as it answered the first time. A volume the proxy cannot ask is taken to
// hold them, so a hold that fails there fails loudly rather than being
// skipped.
func (v *ProxyVolume) HoldsRoots() bool {
	return v.holdsRoots()
}

// loadHoldsRoots asks the remote volume whether it holds roots.
func (v *ProxyVolume) loadHoldsRoots() bool {
	resp, err := v.client.GetHoldsRoots(context.Background(), &volume_rpc.GetHoldsRootsRequest{})
	return err != nil || resp.GetHoldsRoots()
}

// PinBucketRoot pins a reader's root on the remote volume. The pin is a
// stream the remote volume holds the pin across, which the release ends, so a
// proxy that goes away drops its pins.
func (v *ProxyVolume) PinBucketRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	// Open the stream, which outlives ctx, and wait for the pin.
	strmCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	strm, err := v.client.HoldPin(strmCtx, &volume_rpc.HoldPinRequest{Ref: ref})
	if err == nil {
		_, err = strm.Recv()
	}
	if err != nil {
		cancel()
		return nil, err
	}
	return func() {
		_ = strm.Close()
		cancel()
	}, nil
}

// OpenStage opens a stage on the remote volume, held by a stream the release
// ends.
func (v *ProxyVolume) OpenStage(ctx context.Context) (string, func(), error) {
	// Open the stream, which outlives ctx, and read the stage's name.
	strmCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	strm, err := v.client.HoldStage(strmCtx, &volume_rpc.HoldStageRequest{})
	var resp *volume_rpc.HoldStageResponse
	if err == nil {
		resp, err = strm.Recv()
	}
	if err != nil {
		cancel()
		return "", nil, err
	}
	return resp.GetStage(), func() {
		_ = strm.Close()
		cancel()
	}, nil
}

// PrepareStagedBlock writes a block the stage holds.
func (v *ProxyVolume) PrepareStagedBlock(ctx context.Context, stage string, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	resp, err := v.client.PrepareStagedBlock(ctx, &volume_rpc.PrepareStagedBlockRequest{Stage: stage, Data: data, Opts: opts})
	if err != nil {
		return nil, false, err
	}
	return resp.GetRef(), resp.GetExisted(), nil
}

// PrepareStagedBlockBatch writes a batch the stage holds.
func (v *ProxyVolume) PrepareStagedBlockBatch(ctx context.Context, stage string, entries []*block.PutBatchEntry) ([]bool, error) {
	resp, err := v.client.PrepareStagedBlockBatch(ctx, &volume_rpc.PrepareStagedBlockBatchRequest{Stage: stage, Entries: volume_rpc.NewPutBatchEntries(entries)})
	if err != nil {
		return nil, err
	}
	return resp.GetExisted(), nil
}

// ReleaseStageRoots drops the stage's holds on refs.
func (v *ProxyVolume) ReleaseStageRoots(ctx context.Context, stage string, refs []*block.BlockRef) error {
	_, err := v.client.ReleaseStageRoots(ctx, &volume_rpc.StageRefsRequest{Stage: stage, Refs: refs})
	return err
}

// HoldStageCompleteRoots makes the stage hold each proved root.
func (v *ProxyVolume) HoldStageCompleteRoots(ctx context.Context, stage string, refs []*block.BlockRef) ([]bool, error) {
	resp, err := v.client.HoldStageCompleteRoots(ctx, &volume_rpc.StageRefsRequest{Stage: stage, Refs: refs})
	if err != nil {
		return nil, err
	}
	return resp.GetComplete(), nil
}

// MarkRootsComplete records completion proofs for stored roots.
func (v *ProxyVolume) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	_, err := v.client.MarkRootsComplete(ctx, &volume_rpc.RefsRequest{Refs: roots})
	return err
}

// RootsComplete reports which roots have a completion proof.
func (v *ProxyVolume) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	resp, err := v.client.RootsComplete(ctx, &volume_rpc.RefsRequest{Refs: refs})
	if err != nil {
		return nil, err
	}
	return resp.GetComplete(), nil
}

// _ is a type assertion
var _ volume.Roots = (*ProxyVolume)(nil)
