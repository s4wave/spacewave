//go:build js

package browser_storage

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/s4wave/spacewave/bldr/storage"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_opfs "github.com/s4wave/spacewave/db/volume/js/opfs"
)

// OpfsStorage implements OPFS-backed browser storage.
type OpfsStorage struct {
	// prefix namespaces persisted volume directories.
	prefix string
}

// NewOpfsStorage constructs an OPFS storage handle.
func NewOpfsStorage(prefix string) *OpfsStorage {
	return &OpfsStorage{prefix: prefix}
}

// GetStorageInfo returns StorageInfo.
func (s *OpfsStorage) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{}
}

// AddFactories adds the factories to the resolver.
func (s *OpfsStorage) AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_opfs.NewFactory(b))
}

// BuildVolumeConfig creates the volume config for the store ID.
func (s *OpfsStorage) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	rootPath := s.prefix + id
	return &volume_opfs.Config{
		RootPath:             rootPath,
		LockPrefix:           rootPath,
		DriverMode:           "auto",
		StorageFormatVersion: volume_opfs.StorageFormatVersion,
		VolumeConfig:         baseVolCtrlConf,
	}, nil
}

// DeleteVolume removes the active OPFS volume while retaining replaced legacy data.
func (s *OpfsStorage) DeleteVolume(id string) error {
	return volume_opfs.DeleteRoot(s.prefix + id)
}

// _ is a type assertion.
var _ storage.Storage = (*OpfsStorage)(nil)
