package kvtx

import (
	"context"

	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

// volumeCoordinator promotes coordination operations without exposing their implementation.
type volumeCoordinator interface {
	coord.Coordinator
}

// Backend configures the coordination and diagnostics of an opened volume.
// Its callbacks run after the ordinary volume initialization has completed.
type Backend struct {
	// Coordinate builds backend coordination over the volume's default coordinator.
	Coordinate func(string, coord.Coordinator) coord.Coordinator
	// Logger reports writes whose block references were omitted.
	Logger *logrus.Entry
	// StorageFlows fills the backend's contribution to volume flow counters.
	StorageFlows func(*volume.VolumeFlows)
}

// NewVolumeWithBackend opens a volume and binds its backend before returning it.
// It takes ownership of closeFn under the same contract as NewVolume.
func NewVolumeWithBackend(
	backend Backend,
	ctx context.Context,
	storeID string,
	keys *store_kvkey.KVKey,
	store kvtx.Store,
	conf *store_kvtx.Config,
	noGenerateKey, noWriteKey bool,
	statsFn StatsFn,
	closeFn func() error,
	deleteFn ...func() error,
) (*Volume, error) {
	// Finish ordinary initialization before installing backend coordination and diagnostics.
	v, err := NewVolume(ctx, storeID, keys, store, conf, noGenerateKey, noWriteKey, statsFn, closeFn, deleteFn...)
	if err != nil {
		return nil, err
	}

	// Bind the backend to the initialized identity and default coordinator.
	if backend.Coordinate != nil {
		v.volumeCoordinator = backend.Coordinate(v.GetID(), v.volumeCoordinator)
	}
	v.le = backend.Logger
	v.storageFlows = backend.StorageFlows
	return v, nil
}
