package volume_rpc_server

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
)

// GetBucketRoot returns the block a named root of a bucket holds.
func (v *ProxyVolume) GetBucketRoot(ctx context.Context, req *volume_rpc.GetBucketRootRequest) (*volume_rpc.GetBucketRootResponse, error) {
	ref, err := v.vol.GetBucketRoot(ctx, req.GetBucketId(), req.GetName())
	if err != nil {
		return nil, err
	}
	return &volume_rpc.GetBucketRootResponse{Ref: ref}, nil
}

// ListBucketRoots streams the named roots of a bucket under a prefix.
func (v *ProxyVolume) ListBucketRoots(req *volume_rpc.ListBucketRootsRequest, strm volume_rpc.SRPCProxyVolume_ListBucketRootsStream) error {
	return v.vol.ListBucketRoots(strm.Context(), req.GetBucketId(), req.GetPrefix(), func(root block.NamedRoot) error {
		return strm.Send(&volume_rpc.ListBucketRootsResponse{Root: volume_rpc.NewNamedRoot(root)})
	})
}

// SetBucketRoots stores blocks and moves named roots of a bucket in one
// transaction of the volume, reporting a moved root or a volume that holds
// no roots in the result, so the client can tell them apart.
func (v *ProxyVolume) SetBucketRoots(ctx context.Context, req *volume_rpc.SetBucketRootsRequest) (*volume_rpc.SetBucketRootsResponse, error) {
	// Convert the request.
	entries := volume_rpc.ToPutBatchEntries(req.GetEntries())
	moves := make([]block.NamedRoot, 0, len(req.GetRoots()))
	for _, root := range req.GetRoots() {
		moves = append(moves, root.ToNamedRoot())
	}

	// Move the roots; a volume that holds none refuses holds.
	err := v.vol.SetBucketRoots(ctx, req.GetBucketId(), entries, moves)

	// Report the refusals the client acts on in the result.
	switch {
	case errors.Is(err, block.ErrRootMoved):
		return &volume_rpc.SetBucketRootsResponse{Result: volume_rpc.SetBucketRootsResult_SET_BUCKET_ROOTS_RESULT_ROOT_MOVED, Name: err.Error()}, nil
	case errors.Is(err, block.ErrRootsUnheld):
		return &volume_rpc.SetBucketRootsResponse{Result: volume_rpc.SetBucketRootsResult_SET_BUCKET_ROOTS_RESULT_UNHELD, Name: err.Error()}, nil
	case err != nil:
		return nil, err
	}
	return &volume_rpc.SetBucketRootsResponse{}, nil
}

// GetRootRetention reports whether the volume holds roots, pins and stages.
func (v *ProxyVolume) GetRootRetention(context.Context, *volume_rpc.GetRootRetentionRequest) (*volume_rpc.GetRootRetentionResponse, error) {
	return &volume_rpc.GetRootRetentionResponse{Supported: v.vol.SupportsRootRetention()}, nil
}

// HoldPin pins a reader's root and holds it until the client ends the stream.
func (v *ProxyVolume) HoldPin(req *volume_rpc.HoldPinRequest, strm volume_rpc.SRPCProxyVolume_HoldPinStream) error {
	// Pin the root and report it held.
	ctx := strm.Context()
	release, err := v.vol.PinBucketRoot(ctx, req.GetRef())
	if err != nil {
		return err
	}
	defer release()
	if err := strm.Send(&volume_rpc.HoldPinResponse{}); err != nil {
		return err
	}

	// Hold it until the stream ends.
	<-ctx.Done()
	return nil
}

// HoldStage opens a stage, names it to the client, and holds it until the
// client ends the stream.
func (v *ProxyVolume) HoldStage(_ *volume_rpc.HoldStageRequest, strm volume_rpc.SRPCProxyVolume_HoldStageStream) error {
	// Open the stage and record it as this service's.
	ctx := strm.Context()
	stage, release, err := v.vol.OpenStage(ctx)
	if err != nil {
		return err
	}
	v.stageMtx.Lock()
	v.stages[stage] = struct{}{}
	v.stageMtx.Unlock()
	defer func() {
		v.stageMtx.Lock()
		delete(v.stages, stage)
		v.stageMtx.Unlock()
		release()
	}()

	// Name it, then hold it until the stream ends.
	if err := strm.Send(&volume_rpc.HoldStageResponse{Stage: stage}); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

// PrepareStagedBlock writes a block a held stage keeps.
func (v *ProxyVolume) PrepareStagedBlock(ctx context.Context, req *volume_rpc.PrepareStagedBlockRequest) (*volume_rpc.PrepareStagedBlockResponse, error) {
	if err := v.checkStage(req.GetStage()); err != nil {
		return nil, err
	}
	ref, existed, err := v.vol.PrepareStagedBlock(ctx, req.GetStage(), req.GetData(), req.GetOpts())
	if err != nil {
		return nil, err
	}
	return &volume_rpc.PrepareStagedBlockResponse{Ref: ref, Existed: existed}, nil
}

// PrepareStagedBlockBatch writes a batch a held stage keeps.
func (v *ProxyVolume) PrepareStagedBlockBatch(ctx context.Context, req *volume_rpc.PrepareStagedBlockBatchRequest) (*volume_rpc.PrepareStagedBlockBatchResponse, error) {
	if err := v.checkStage(req.GetStage()); err != nil {
		return nil, err
	}
	existed, err := v.vol.PrepareStagedBlockBatch(ctx, req.GetStage(), volume_rpc.ToPutBatchEntries(req.GetEntries()))
	if err != nil {
		return nil, err
	}
	return &volume_rpc.PrepareStagedBlockBatchResponse{Existed: existed}, nil
}

// ReleaseStageRoots drops a held stage's holds on blocks.
func (v *ProxyVolume) ReleaseStageRoots(ctx context.Context, req *volume_rpc.StageRefsRequest) (*volume_rpc.ReleaseStageRootsResponse, error) {
	if err := v.checkStage(req.GetStage()); err != nil {
		return nil, err
	}
	if err := v.vol.ReleaseStageRoots(ctx, req.GetStage(), req.GetRefs()); err != nil {
		return nil, err
	}
	return &volume_rpc.ReleaseStageRootsResponse{}, nil
}

// HoldStageCompleteRoots makes a held stage hold each proved root.
func (v *ProxyVolume) HoldStageCompleteRoots(ctx context.Context, req *volume_rpc.StageRefsRequest) (*volume_rpc.RootsCompleteResponse, error) {
	if err := v.checkStage(req.GetStage()); err != nil {
		return nil, err
	}
	complete, err := v.vol.HoldStageCompleteRoots(ctx, req.GetStage(), req.GetRefs())
	if err != nil {
		return nil, err
	}
	return &volume_rpc.RootsCompleteResponse{Complete: complete}, nil
}

// MarkRootsComplete records completion proofs for stored roots.
func (v *ProxyVolume) MarkRootsComplete(ctx context.Context, req *volume_rpc.RefsRequest) (*volume_rpc.MarkRootsCompleteResponse, error) {
	if err := v.vol.MarkRootsComplete(ctx, req.GetRefs()); err != nil {
		return nil, err
	}
	return &volume_rpc.MarkRootsCompleteResponse{}, nil
}

// RootsComplete reports which roots have a completion proof.
func (v *ProxyVolume) RootsComplete(ctx context.Context, req *volume_rpc.RefsRequest) (*volume_rpc.RootsCompleteResponse, error) {
	complete, err := v.vol.RootsComplete(ctx, req.GetRefs())
	if err != nil {
		return nil, err
	}
	return &volume_rpc.RootsCompleteResponse{Complete: complete}, nil
}

// checkStage refuses a stage no HoldStage stream of this service holds.
func (v *ProxyVolume) checkStage(stage string) error {
	// Look the stage up among the held ones.
	v.stageMtx.Lock()
	_, held := v.stages[stage]
	v.stageMtx.Unlock()
	if !held {
		return errors.Wrap(block.ErrStageReleased, stage)
	}
	return nil
}
