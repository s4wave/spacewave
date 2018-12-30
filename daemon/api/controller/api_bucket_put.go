package api_controller

import (
	"context"
	"regexp"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/daemon/api"
	"github.com/pkg/errors"
)

// PutBlock requests the system ingest a block.
func (a *API) PutBlock(
	req *api.PutBlockRequest,
	serv api.HydraDaemonService_PutBlockServer,
) error {
	ctx := serv.Context()
	if err := req.Validate(); err != nil {
		return err
	}
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

	errCh := make(chan error, 1)
	putErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	// TODO this is awkward
	var doneWg sync.WaitGroup
	added := func(aval directive.AttachedValue) {
		val, ok := aval.GetValue().(bucket.BuildBucketAPIValue)
		if !ok {
			return
		}
		doneWg.Add(1)
		go func() {
			defer doneWg.Done()
			e, err := val.PutBlock(req.GetData(), req.GetPutOpts())
			if err != nil {
				putErr(err)
				return
			}
			_ = serv.Send(&api.PutBlockResponse{
				Event: e,
			})
		}()
	}
	dir, err := bucket.NewBuildBucketAPI(
		bucket.WithBucketID(req.GetBucketId()),
		bucket.WithVolumeIDRegex(volumeIdRe),
	)
	if err != nil {
		return err
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
		return err
	}
	defer ref.Release()
	di.AddIdleCallback(reqCtxCancel)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-reqCtx.Done():
		doneWg.Wait()
		return nil
	case err := <-errCh:
		return err
	}
}
