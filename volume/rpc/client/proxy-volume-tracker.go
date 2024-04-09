package volume_rpc_client

import (
	"context"
	"sync/atomic"
	"time"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	rpc_block "github.com/aperturerobotics/hydra/block/rpc"
	rpc_bucket "github.com/aperturerobotics/hydra/bucket/store/rpc"
	rpc_mqueue "github.com/aperturerobotics/hydra/mqueue/rpc"
	rpc_object "github.com/aperturerobotics/hydra/object/rpc"
	"github.com/aperturerobotics/hydra/volume"
	volume_rpc "github.com/aperturerobotics/hydra/volume/rpc"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/sirupsen/logrus"
)

// proxyVolumeTracker tracks a ProxyVolume.
type proxyVolumeTracker struct {
	// c is the controller
	c *Controller
	// le is the logger
	le *logrus.Entry
	// volumeID is the volume identifier.
	volumeID string
	// proxyVolCtr contains the proxy volume controller
	proxyVolCtr *ccontainer.CContainer[*ProxyVolumeController]
}

// newProxyVolumeTracker constructs a new proxy volume tracker routine.
func (c *Controller) newProxyVolumeTracker(key string) (keyed.Routine, *proxyVolumeTracker) {
	tr := &proxyVolumeTracker{
		c:           c,
		le:          c.le.WithField("volume-id", key),
		volumeID:    key,
		proxyVolCtr: ccontainer.NewCContainer[*ProxyVolumeController](nil),
	}
	return tr.execute, tr
}

// execute executes the proxy volume tracker.
// manages retry + backoff if something goes wrong.
func (t *proxyVolumeTracker) execute(ctx context.Context) error {
	le := t.le

	backoffOpts := t.c.cc.GetBackoff()
	if backoffOpts.GetEmpty() {
		backoffOpts = &backoff.Backoff{
			BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
			Exponential: &backoff.Exponential{
				InitialInterval: 250,
				Multiplier:      2,
				MaxInterval:     5000,
			},
		}
	}

	bo := backoffOpts.Construct()
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}

		le.Debug("starting proxy volume controller")
		err := t.executeOnce(ctx, le, bo.Reset)
		t.proxyVolCtr.SetValue(nil)
		if err != nil && err != context.Canceled {
			nextBo := bo.NextBackOff()
			le.
				WithError(err).
				WithField("backoff", nextBo.String()).
				Warn("proxy volume controller exited with error")
			if nextBo == backoff.Stop {
				return err
			}
			afterTimer := time.NewTimer(nextBo)
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-afterTimer.C:
			}
		} else {
			le.Debug("proxy volume controller exited without an error")
		}
	}
}

// executeOnce executes the proxy volume controller once.
func (t *proxyVolumeTracker) executeOnce(ctx context.Context, le *logrus.Entry, success func()) error {
	// build client for the AccessVolumes service
	volumeID := t.volumeID
	clientSet, _, clientSetRef, err := bifrost_rpc.ExLookupRpcClientSet(
		ctx,
		t.c.bus,
		t.c.cc.GetServiceId(),
		t.c.cc.GetClientId(),
		true,
		nil,
	)
	if err != nil {
		return err
	}
	defer clientSetRef.Release()

	accessVolumes := volume_rpc.NewSRPCAccessVolumesClientWithServiceID(clientSet, t.c.cc.GetServiceId())
	openStreamFn := rpcstream.NewRpcStreamOpenStream(accessVolumes.VolumeRpc, volumeID, false)
	volClient := srpc.NewClient(openStreamFn)

	// call WatchVolumeInfo to detect when we need to rebuild the volume controller
	infoClient, err := accessVolumes.WatchVolumeInfo(ctx, &volume_rpc.WatchVolumeInfoRequest{
		VolumeId: volumeID,
	})
	if err != nil {
		return err
	}
	defer infoClient.Close()

	// start a routine to watch the volume info
	volInfoCtr := ccontainer.NewCContainer[*volume.VolumeInfo](nil)
	errCh := make(chan error, 5)
	go func() {
		for {
			infoResp, err := infoClient.Recv()
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			volInfo := infoResp.GetVolumeInfo()
			if infoResp.GetNotFound() || volInfo.GetVolumeId() == "" {
				volInfo = nil
			}
			_ = volInfoCtr.SwapValue(func(oldInfo *volume.VolumeInfo) *volume.VolumeInfo {
				if oldInfo.EqualVT(volInfo) {
					// no change
					return oldInfo
				}
				return volInfo
			})
		}
	}()

	// routine
	var currVolInfo atomic.Pointer[volume.VolumeInfo]
	startRoutine := func(ctx context.Context, volInfo *volume.VolumeInfo) {
		// lookup configured volume id aliases
		volumeIDAlias := t.c.cc.GetVolumeAliases()[volInfo.GetVolumeId()]
		err := t.execProxyVolumeController(ctx, volInfo, volumeIDAlias.GetFrom(), volClient, success)
		if err != nil && currVolInfo.Load() == volInfo {
			select {
			case errCh <- err:
			default:
			}
		}
	}

	// routine cancel
	var proxyCtxCancel context.CancelFunc
	defer func() {
		if proxyCtxCancel != nil {
			proxyCtxCancel()
		}
	}()

	// wait for volume info changes
	// if the info changes, restart the routine.
	for {
		prevVolInfo := currVolInfo.Load()
		updVolInfo, err := volInfoCtr.WaitValueChange(ctx, prevVolInfo, errCh)
		currVolInfo.Store(updVolInfo)
		if proxyCtxCancel != nil {
			proxyCtxCancel()
		}
		if err != nil {
			return err
		}
		var proxyCtx context.Context
		proxyCtx, proxyCtxCancel = context.WithCancel(ctx)
		go startRoutine(proxyCtx, updVolInfo)
	}
}

// execProxyVolumeController executes the ProxyVolumeController.
func (t *proxyVolumeTracker) execProxyVolumeController(
	ctx context.Context,
	volumeInfo *volume.VolumeInfo,
	volumeIDAlias []string,
	volClient srpc.Client,
	success func(),
) error {
	proxyVolCtrl := NewProxyVolumeController(
		t.c.bus,
		t.le,
		volumeInfo,
		volumeIDAlias,
		volume_rpc.NewSRPCProxyVolumeClient(volClient),
		rpc_block.NewSRPCBlockStoreClient(volClient),
		rpc_bucket.NewSRPCBucketStoreClient(volClient),
		rpc_object.NewSRPCObjectStoreClient(volClient),
		rpc_mqueue.NewSRPCMqueueStoreClient(volClient),
	)

	select {
	case <-ctx.Done():
		return context.Canceled
	default:
	}

	t.le.Debug("proxy volume controller ready")
	t.proxyVolCtr.SetValue(proxyVolCtrl)
	success()
	return t.c.bus.ExecuteController(ctx, proxyVolCtrl)
}
