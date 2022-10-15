package rpc_volume_client

import (
	"context"
	"sync/atomic"

	bldr_rpc "github.com/aperturerobotics/bldr/rpc"
	rpc_block "github.com/aperturerobotics/bldr/rpc/block"
	rpc_bucket "github.com/aperturerobotics/bldr/rpc/bucket"
	rpc_mqueue "github.com/aperturerobotics/bldr/rpc/mqueue"
	rpc_object "github.com/aperturerobotics/bldr/rpc/object"
	rpc_volume "github.com/aperturerobotics/bldr/rpc/volume"
	"github.com/aperturerobotics/controllerbus/util/ccontainer"
	"github.com/aperturerobotics/controllerbus/util/keyed"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
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
func (t *proxyVolumeTracker) execute(ctx context.Context) error {
	le, volumeID := t.le, t.volumeID
	le.Debug("starting proxy volume controller")

	// build client for the AccessVolumes service
	clientSet, clientSetRef, err := bldr_rpc.ExLookupRpcClientSet(
		ctx,
		t.c.bus,
		t.c.cc.GetServiceId(),
		t.c.cc.GetClientId(),
	)
	if err != nil {
		return err
	}
	defer clientSetRef.Release()

	accessVolumes := rpc_volume.NewSRPCAccessVolumesClient(clientSet)
	openStreamFn := rpcstream.NewRpcStreamOpenStream(accessVolumes.VolumeRpc, volumeID)
	volClient := srpc.NewClient(openStreamFn)

	// call WatchVolumeInfo to detect when we need to rebuild the volume controller
	infoClient, err := accessVolumes.WatchVolumeInfo(ctx, &rpc_volume.WatchVolumeInfoRequest{
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
			if volInfoCtr.GetValue().EqualVT(volInfo) {
				// no change
				continue
			}
			volInfoCtr.SetValue(volInfo)
		}
	}()

	// routine cancel
	var currVolInfo atomic.Pointer[volume.VolumeInfo]
	var proxyCtxCancel context.CancelFunc
	defer func() {
		if proxyCtxCancel != nil {
			proxyCtxCancel()
		}
	}()

	// routine
	startRoutine := func(ctx context.Context, volInfo *volume.VolumeInfo) {
		err := t.execProxyVolumeController(ctx, volInfo, volClient)
		if err != nil && currVolInfo.Load() == volInfo {
			select {
			case errCh <- err:
			default:
			}
		}
	}

	// wait for volume info changes
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

// waitProxyVolumeCtrl waits for the proxy volume controller to be ready.
func (t *proxyVolumeTracker) waitProxyVolumeCtrl(ctx context.Context) (*ProxyVolumeController, error) {
	val, err := t.proxyVolCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return val, nil
}

// execProxyVolumeController executes the ProxyVolumeController.
func (t *proxyVolumeTracker) execProxyVolumeController(
	ctx context.Context,
	volumeInfo *volume.VolumeInfo,
	volClient srpc.Client,
) error {
	proxyVolCtrl := NewProxyVolumeController(
		t.le,
		volumeInfo,
		rpc_volume.NewSRPCProxyVolumeClient(volClient),
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
	return t.c.bus.ExecuteController(ctx, proxyVolCtrl)
}
