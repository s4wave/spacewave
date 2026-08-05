package hydra_api

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/volume"
)

// ApplyBucketConfig requests the system ingest a bucket config.
func (a *API) ApplyBucketConfig(
	req *ApplyBucketConfigRequest,
	serv SRPCHydraDaemonService_ApplyBucketConfigStream,
) error {
	// Validate and resolve the requested bucket configuration.
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

	// Watch the configuration and stream each result.
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

	// Complete the stream when the resolver becomes idle.
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
		if emittedAny.Load() {
			handleErr(nil)
		}
	})()

	// Wait for cancellation or the resolver result.
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// ListBuckets lists basic bucket information
func (a *API) ListBuckets(
	ctx context.Context,
	req *volume.ListBucketsRequest,
) (*ListBucketsResponse, error) {
	// Collect bucket values from the controller bus.
	bucketInfos, _, ref, err := bus.ExecCollectValues[*volume.ListBucketsValue](ctx, a.bus, req, false, nil)
	if err != nil {
		return nil, err
	}
	ref.Release()

	// Return the collected bucket information.
	return &ListBucketsResponse{Buckets: bucketInfos}, nil
}
