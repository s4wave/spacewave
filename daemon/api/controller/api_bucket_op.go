package hydra_api_controller

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/event"
	api "github.com/aperturerobotics/hydra/daemon/api"
	"github.com/aperturerobotics/hydra/node"
	"github.com/aperturerobotics/hydra/volume"
)

// BucketOp performs a bucket operation.
func (a *API) BucketOp(
	ctx context.Context,
	req *api.BucketOpRequest,
) (*api.BucketOpResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	bk, rel, err := a.startBucketOp(ctx, req.GetBucketOpArgs())
	if err != nil {
		return nil, err
	}
	defer rel()
	resp := &api.BucketOpResponse{}
	switch req.GetOp() {
	case api.BucketOp_BucketOp_BLOCK_PUT:
		pe, err := bk.PutBlock(req.GetData(), req.GetPutOpts())
		if err != nil {
			return nil, err
		}
		resp.Event = &bucket_event.Event{
			EventType: bucket_event.EventType_EventType_PUT_BLOCK,
			PutBlock:  pe,
		}
	case api.BucketOp_BucketOp_BLOCK_GET:
		dat, ok, err := bk.GetBlock(req.GetBlockRef())
		if err != nil {
			return nil, err
		}
		resp.Data = dat
		resp.Found = ok
	case api.BucketOp_BucketOp_BLOCK_RM:
		if err := bk.RmBlock(req.GetBlockRef()); err != nil {
			return nil, err
		}
	case api.BucketOp_BucketOp_UNKNOWN:
		fallthrough
	default:
		return nil, errors.New("unknown bucket op code")
	}

	return resp, nil
}

// startBucketOp starts a bucket operation.
func (a *API) startBucketOp(
	ctx context.Context,
	args *volume.BucketOpArgs,
) (bk bucket.Bucket, rel func(), err error) {
	return node.StartBucketRWOperation(ctx, a.bus, args)
}
