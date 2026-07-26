package sobject_world_engine

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/volume"
)

type worldEngineLeaseVolumeProvider interface {
	GetBackingVolume() volume.Volume
}

func (c *Controller) acquireWorldEngineLease(
	ctx context.Context,
	so sobject.SharedObject,
) (volume.WorldEngineLease, error) {
	volumeProvider, ok := so.(worldEngineLeaseVolumeProvider)
	if !ok {
		return nil, errors.New("shared object does not expose its backing volume")
	}
	vol := volumeProvider.GetBackingVolume()
	if vol == nil {
		return nil, errors.New("shared object backing volume is nil")
	}

	return vol.AcquireWorldEngineLease(ctx, so.GetSharedObjectID())
}

func watchWorldEngineLease(
	ctx context.Context,
	lease volume.WorldEngineLease,
	cancel context.CancelFunc,
) {
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
