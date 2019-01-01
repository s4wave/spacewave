package api_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/pkg/errors"
)

// GetBlock requests the system get a block.
func (a *API) GetBlock(
	ctx context.Context,
	req *api.GetBlockRequest,
) (*api.GetBlockResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	volumeIdRe, err := req.ParseVolumeIDRe()
	if err != nil {
		return nil, errors.Wrap(err, "volume id regex parse")
	}

	reqCtx, reqCtxCancel := context.WithCancel(ctx)
	defer reqCtxCancel()

	errCh := make(chan error, 1)
	putErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	// TODO this is awkward
	var doneWg sync.WaitGroup
	respMtx := sync.Mutex{}
	resp := &api.GetBlockResponse{
		BlockRef: req.GetBlockRef(),
		BucketId: req.GetBucketId(),
	}
	added := func(aval directive.AttachedValue) {
		val, ok := aval.GetValue().(bucket.BuildBucketAPIValue)
		if !ok {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		doneWg.Add(1)
		go func() {
			defer doneWg.Done()
			dat, ok, err := val.GetBlock(req.GetBlockRef())
			if err != nil {
				putErr(err)
				return
			}
			respMtx.Lock()
			if !resp.Found {
				resp.Found = ok
				resp.Data = dat
				resp.VolumeId = val.GetVolumeId()
			}
			respMtx.Unlock()
		}()
	}
	dir, err := bucket.NewBuildBucketAPI(
		bucket.WithBucketID(req.GetBucketId()),
		bucket.WithVolumeIDRegex(volumeIdRe),
	)
	if err != nil {
		return nil, err
	}
	di, ref, err := a.bus.AddDirective(
		dir,
		bus.NewCallbackHandler(
			added,
			nil,
			reqCtxCancel,
		),
	)
	if err != nil {
		return nil, err
	}
	defer ref.Release()
	di.AddIdleCallback(reqCtxCancel)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-reqCtx.Done():
		doneWg.Wait()
		return resp, nil
	case err := <-errCh:
		return nil, err
	}
}
