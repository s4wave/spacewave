package hydra_api

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_event "github.com/s4wave/spacewave/db/bucket/event"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// BucketOp performs a bucket operation.
func (a *API) BucketOp(
	ctx context.Context,
	req *BucketOpRequest,
) (*BucketOpResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	bk, rel, err := a.startBucketOp(ctx, req.GetBucketOpArgs())
	if err != nil {
		return nil, err
	}
	defer rel()
	resp := &BucketOpResponse{}
	switch req.GetOp() {
	case BucketOp_BucketOp_BLOCK_PUT:
		ref, existed, err := bk.PutBlock(ctx, req.GetData(), req.GetPutOpts())
		if err != nil {
			return nil, err
		}
		resp.Event = &bucket_event.Event{
			EventType: bucket_event.EventType_EventType_PUT_BLOCK,
			PutBlock: &bucket_event.PutBlock{
				BlockCommon: &bucket_event.BlockCommon{
					BucketId:      req.GetBucketOpArgs().GetBucketId(),
					VolumeId:      req.GetBucketOpArgs().GetVolumeId(),
					BucketConfRev: bk.GetBucketConfig().GetRev(),
					BlockRef:      ref,
				},
			},
		}
		_ = existed
	case BucketOp_BucketOp_BLOCK_GET:
		dat, ok, err := bk.GetBlock(ctx, req.GetBlockRef())
		if err != nil {
			return nil, err
		}
		resp.Data = dat
		resp.Found = ok
	case BucketOp_BucketOp_BLOCK_RM:
		if err := bk.RmBlock(ctx, req.GetBlockRef()); err != nil {
			return nil, err
		}
		resp.Event = &bucket_event.Event{
			EventType: bucket_event.EventType_EventType_RM_BLOCK,
			RmBlock: &bucket_event.RmBlock{
				BlockCommon: &bucket_event.BlockCommon{
					BucketId:      req.GetBucketOpArgs().GetBucketId(),
					VolumeId:      req.GetBucketOpArgs().GetVolumeId(),
					BucketConfRev: bk.GetBucketConfig().GetRev(),
					BlockRef:      req.GetBlockRef(),
				},
			},
		}
	case BucketOp_BucketOp_UNKNOWN:
		fallthrough
	default:
		return nil, errors.New("unknown bucket op code")
	}

	return resp, nil
}

// startBucketOp starts a bucket operation.
func (a *API) startBucketOp(
	ctx context.Context,
	args *bucket.BucketOpArgs,
) (bk bucket.Bucket, rel func(), err error) {
	return bucket_lookup.StartBucketRWOperation(ctx, a.bus, args)
}
