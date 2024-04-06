package hydra_api

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/volume"
)

// ApplyBucketConfig requests the system ingest a bucket config.
func (a *API) ApplyBucketConfig(
	req *ApplyBucketConfigRequest,
	serv SRPCHydraDaemonService_ApplyBucketConfigStream,
) error {
	ctx := serv.Context()
	if err := req.Validate(); err != nil {
		return err
	}

	errCh := make(chan error, 1)
	handleErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	applyBucketConf, err := req.ToApplyBucketConfig()
	if err != nil {
		return err
	}

	var emittedAny atomic.Bool
	di, diRef, err := bus.ExecWatchEffect(
		func(val directive.TypedAttachedValue[*bucket.ApplyBucketConfigResult]) func() {
			emittedAny.Store(true)
			if err := serv.Send(&ApplyBucketConfigResponse{
				ApplyConfResult: val.GetValue(),
			}); err != nil {
				handleErr(err)
			}
			return nil
		},
		a.bus,
		applyBucketConf,
	)
	if err != nil {
		return err
	}
	defer diRef.Release()

	defer di.AddIdleCallback(func(isIdle bool, resolverErrs []error) {
		if !isIdle {
			return
		}

		for _, err := range resolverErrs {
			if err != nil && err != context.Canceled {
				handleErr(err)
				return
			}
		}

		// handle idle with success if at least one value emitted
		if emittedAny.Load() {
			handleErr(nil)
		}
	})

	select {
	case <-ctx.Done():
		return nil
	case <-errCh:
		return err
	}
}

// ListBuckets lists basic bucket information
func (a *API) ListBuckets(
	ctx context.Context,
	req *volume.ListBucketsRequest,
) (*ListBucketsResponse, error) {

	bucketInfos, _, ref, err := bus.ExecCollectValues[*volume.ListBucketsValue](ctx, a.bus, req, false, nil)
	if err != nil {
		return nil, err
	}
	ref.Release()

	return &ListBucketsResponse{Buckets: bucketInfos}, nil
}
