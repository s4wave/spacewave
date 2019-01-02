package api_controller

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/aperturerobotics/hydra/volume"
)

// GetBlock requests the system get a block.
func (a *API) GetBlock(
	ctx context.Context,
	req *api.GetBlockRequest,
) (*api.GetBlockResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	bh, err := volume.StartBucketOperation(ctx, a.bus, req.GetBucketOpArgs())
	if err != nil {
		return nil, err
	}
	if !bh.GetExists() {
		return nil, errors.New("bucket not found")
	}
	defer bh.Close()

	dat, ok, err := bh.GetBucket().GetBlock(req.GetBlockRef())
	if err != nil {
		return nil, err
	}
	return &api.GetBlockResponse{
		Found: ok,
		Data:  dat,
	}, nil
}
