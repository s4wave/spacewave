package storage_inmem

import (
	"github.com/s4wave/spacewave/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
)

// InmemStorage provides in-memory storage.
type InmemStorage struct{}

// NewInmemStorage constructs storage from CLI args.
func NewInmemStorage() *InmemStorage {
	return &InmemStorage{}
}

// GetStorageInfo returns StorageInfo.
func (s *InmemStorage) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{}
}

// AddFactories adds the factories to the resolver.
func (s *InmemStorage) AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_kvtxinmem.NewFactory(b))
}

// BuildVolumeConfig creates the volume config for the store ID.
// Returns nil if the storage cannot produce Volume.
// baseVolCtrlConf can be nil
func (s *InmemStorage) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	return &volume_kvtxinmem.Config{VolumeConfig: baseVolCtrlConf}, nil
}

// DeleteVolume is a no-op for in-memory storage.
func (s *InmemStorage) DeleteVolume(id string) error {
	return nil
}

// _ is a type assertion
var _ storage.Storage = ((*InmemStorage)(nil))
