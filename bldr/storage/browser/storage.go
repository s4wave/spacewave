//go:build js

package browser_storage

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/s4wave/spacewave/bldr/storage"
	volume_browser "github.com/s4wave/spacewave/db/volume/browser"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
)

// volumeDir holds every browser volume, apart from the data earlier storage
// formats left in the origin.
const volumeDir = "volumes/"

// Storage stores each volume in the browser volume, on OPFS or IndexedDB.
type Storage struct {
	// prefix namespaces the volume names.
	prefix string
}

// BuildStorage builds the browser storage. prefix namespaces the volumes.
func BuildStorage(_ bus.Bus, prefix string) []storage.Storage {
	return []storage.Storage{&Storage{prefix: prefix}}
}

// GetStorageInfo returns StorageInfo.
func (s *Storage) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{}
}

// AddFactories adds the factories to the resolver.
func (s *Storage) AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_browser.NewFactory(b))
}

// BuildVolumeConfig creates the volume config for the store ID.
func (s *Storage) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	return &volume_browser.Config{
		Name:         s.volumeName(id),
		VolumeConfig: baseVolCtrlConf,
	}, nil
}

// DeleteVolume deletes the volume for the store ID.
func (s *Storage) DeleteVolume(id string) error {
	return volume_browser.Delete(s.volumeName(id))
}

// volumeName returns the browser volume name of the store ID.
func (s *Storage) volumeName(id string) string {
	return volumeDir + s.prefix + id
}

// _ is a type assertion
var _ storage.Storage = (*Storage)(nil)
