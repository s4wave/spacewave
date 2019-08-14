package hydra_api_controller

import (
	"context"
	"regexp"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	api "github.com/aperturerobotics/hydra/daemon/api"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/pkg/errors"
)

// PutBucketConfig requests the system ingest a bucket config.
func (a *API) PutBucketConfig(
	req *api.PutBucketConfigRequest,
	serv api.HydraDaemonService_PutBucketConfigServer,
) error {
	ctx := serv.Context()
	var volumeIdRe *regexp.Regexp
	if req.GetVolumeIdRegex() != "" {
		var err error
		volumeIdRe, err = regexp.Compile(req.GetVolumeIdRegex())
		if err != nil {
			return errors.Wrap(err, "volume id regex parse")
		}
	}

	reqCtx, reqCtxCancel := context.WithCancel(ctx)
	defer reqCtxCancel()

	added := func(aval directive.AttachedValue) {
		val, ok := aval.GetValue().(*bucket.ApplyBucketConfigValue)
		if !ok {
			return
		}
		_ = serv.Send(&api.PutBucketConfigResponse{
			ApplyConfResult: val,
		})
	}
	di, ref, err := a.bus.AddDirective(
		bucket.NewApplyBucketConfig(req.GetConfig(), volumeIdRe),
		bus.NewCallbackHandler(
			added,
			nil,
			reqCtxCancel,
		),
	)
	if err != nil {
		return err
	}
	defer ref.Release()
	di.AddIdleCallback(reqCtxCancel)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-reqCtx.Done():
		return nil
	}
}

// ListBuckets lists basic bucket information
func (a *API) ListBuckets(
	ctx context.Context,
	req *volume.ListBucketsRequest,
) (*api.ListBucketsResponse, error) {
	var bucketInfos []*volume.ListBucketsValue
	reqCtx, reqCtxCancel := context.WithCancel(ctx)
	defer reqCtxCancel()
	di, diRef, err := a.bus.AddDirective(
		req,
		bus.NewCallbackHandler(func(av directive.AttachedValue) {
			v, ok := av.GetValue().(*volume.ListBucketsValue)
			if !ok {
				return
			}
			bucketInfos = append(bucketInfos, v)
		}, nil, reqCtxCancel),
	)
	if err != nil {
		return nil, err
	}
	defer diRef.Release()
	di.AddIdleCallback(reqCtxCancel)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-reqCtx.Done():
	}

	return &api.ListBucketsResponse{
		Buckets: bucketInfos,
	}, nil
}
