package storage_inmem

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/bldr/storage"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
)

// InmemStorage provides process-local storage. Each volume keeps its data
// until the volume is deleted or the storage is released, so a restarted
// volume controller reopens the same data as it would from disk.
type InmemStorage struct {
	// storageID identifies the storage in LookupStorage.
	storageID string
	// mtx guards volumes.
	mtx sync.Mutex
	// volumes holds the store behind each volume id.
	volumes map[string]*inmemVolume
}

// inmemVolume is the store behind one volume id.
type inmemVolume struct {
	// store holds the volume data.
	store store_kvtx.Store
	// lease admits one open volume at a time, as a file lock does for
	// on-disk storage.
	lease csync.Mutex
}

// NewInmemStorage constructs in-memory storage registered as storageID.
func NewInmemStorage(storageID string) *InmemStorage {
	return &InmemStorage{
		storageID: storageID,
		volumes:   make(map[string]*inmemVolume),
	}
}

// GetStorageInfo returns StorageInfo.
func (s *InmemStorage) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{}
}

// AddFactories adds the factories to the resolver.
func (s *InmemStorage) AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(NewVolumeFactory(b))
}

// BuildVolumeConfig creates the volume config for the volume id.
// baseVolCtrlConf can be nil.
func (s *InmemStorage) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	return &VolumeConfig{
		StorageId:       s.storageID,
		StorageVolumeId: id,
		VolumeConfig:    baseVolCtrlConf,
	}, nil
}

// DeleteVolume drops the data for the volume id.
// The volume must be closed before calling this.
func (s *InmemStorage) DeleteVolume(id string) error {
	s.mtx.Lock()
	delete(s.volumes, id)
	s.mtx.Unlock()
	return nil
}

// openVolume leases the store for the volume id, creating it when absent and
// waiting while another volume holds it. The returned release ends the lease.
func (s *InmemStorage) openVolume(ctx context.Context, id string) (store_kvtx.Store, func(), error) {
	s.mtx.Lock()
	vol := s.volumes[id]
	if vol == nil {
		vol = &inmemVolume{store: kvtx_inmem.NewStore()}
		s.volumes[id] = vol
	}
	s.mtx.Unlock()

	release, err := vol.lease.Lock(ctx)
	if err != nil {
		return nil, nil, err
	}
	return vol.store, release, nil
}

// _ is a type assertion
var _ storage.Storage = (*InmemStorage)(nil)
