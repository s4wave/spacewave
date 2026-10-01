package sobject_world_engine

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/volume"
)

type worldEngineLeaseVolumeProvider interface {
	GetBackingVolume() volume.Volume
}

// acquireWorldEngineLease waits for the write lease on the shared object's
// storage, which excludes concurrent World writers. It reports whether the
// lease signals its loss.
func (c *Controller) acquireWorldEngineLease(
	ctx context.Context,
	so sobject.SharedObject,
) (coord.WriteLease, bool, error) {
	// Resolve the shared object's backing volume.
	volumeProvider, ok := so.(worldEngineLeaseVolumeProvider)
	if !ok {
		return nil, false, errors.New("shared object does not expose its backing volume")
	}
	vol := volumeProvider.GetBackingVolume()
	if vol == nil {
		return nil, false, errors.New("shared object backing volume is nil")
	}

	// Check that the volume supports keyed write leases.
	scope := coord.Scope{
		VolumeID: vol.GetID(),
		Key:      so.GetSharedObjectID(),
	}
	capability, err := vol.Capability(ctx, scope)
	if err != nil {
		return nil, false, err
	}
	if capability == nil || !capability.Supported {
		return nil, false, coord.ErrUnsupported
	}

	// A predecessor engine for this object releases the lease as it stops.
	lease, err := vol.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		return nil, false, err
	}
	return lease, capability.DetectsLoss, nil
}

func watchWorldEngineLease(
	ctx context.Context,
	lease coord.WriteLease,
	detectsLoss bool,
	cancel context.CancelFunc,
) {
	if !detectsLoss {
		return
	}
	go func() {
		select {
		case <-ctx.Done():
		case <-lease.Done():
			if lease.Err() != nil {
				cancel()
			}
		}
	}()
}
