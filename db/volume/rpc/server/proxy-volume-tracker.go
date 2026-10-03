package volume_rpc_server

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/db/volume"
)

// proxyVolumeTracker tracks a ProxyVolume.
type proxyVolumeTracker struct {
	// c is the controller
	c *Controller
	// volumeID is the volume identifier.
	volumeID string
	// proxyVolCtr contains the proxy volume
	// set when the mux is ready to use
	proxyVolCtr *ccontainer.CContainer[*ProxyVolume]
	// muxCtr contains the srpc mux.
	// set when the mux is ready to use
	muxCtr *ccontainer.CContainer[*srpc.Mux]
}

// newProxyVolumeTracker constructs a new proxy volume tracker routine.
func (c *Controller) newProxyVolumeTracker(key string) (keyed.Routine, *proxyVolumeTracker) {
	tr := &proxyVolumeTracker{
		c:           c,
		volumeID:    key,
		proxyVolCtr: ccontainer.NewCContainer[*ProxyVolume](nil),
		muxCtr:      ccontainer.NewCContainer[*srpc.Mux](nil),
	}
	return tr.execute, tr
}

// execute executes the proxy volume tracker.
func (t *proxyVolumeTracker) execute(ctx context.Context) error {
	// Identify the tracked volume in proxy server logs.
	volumeID := t.volumeID
	le := t.c.le.WithField("volume-id", volumeID)

	// Watch the volume lookup for the lifetime of the tracker.
	le.Debug("starting proxy volume")
	valCh, _, valRef, err := bus.ExecOneOffWatchCh[volume.LookupVolumeValue](
		t.c.bus,
		volume.NewLookupVolume(volumeID, ""),
	)
	if err != nil {
		return err
	}
	defer valRef.Release()

	// Replace the published proxy services whenever the volume changes.
	var vol volume.Volume
WaitLoop:
	for {
		// Wait for the next volume lookup value or tracker cancellation.
		select {
		case <-ctx.Done():
			return context.Canceled
		case av := <-valCh:
			var lvv volume.LookupVolumeValue
			if av != nil {
				lvv = av.GetValue()
			}
			if vol == lvv {
				continue WaitLoop
			}
			vol = lvv
		}

		// Clear published services when the volume lookup becomes unavailable.
		if vol == nil {
			t.proxyVolCtr.SetValue(nil)
			t.muxCtr.SetValue(nil)
			continue
		}

		// Register and publish the services for the current volume.
		mux := srpc.NewMux()
		proxyVol := NewProxyVolume(ctx, vol, t.c.cc.GetExposePrivateKey())
		if err := RegisterProxyVolume(mux, proxyVol); err != nil {
			return err
		}
		le.Debug("proxy volume ready")
		t.proxyVolCtr.SetValue(proxyVol)
		t.muxCtr.SetValue(&mux)
	}
}

// waitMux waits for the mux to be ready.
func (t *proxyVolumeTracker) waitMux(ctx context.Context) (srpc.Mux, error) {
	val, err := t.muxCtr.WaitValue(ctx, nil)
	if err != nil || val == nil {
		return nil, err
	}
	return *val, nil
}
