package api_controller

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/aperturerobotics/hydra/volume"
)

// PutBlock requests the system ingest a block.
func (a *API) PutBlock(
	ctx context.Context,
	req *api.PutBlockRequest,
) (*api.PutBlockResponse, error) {
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

	e, err := bh.GetBucket().PutBlock(req.GetData(), req.GetPutOpts())
	if err != nil {
		return nil, err
	}
	return &api.PutBlockResponse{
		Event: e,
	}, nil
}
