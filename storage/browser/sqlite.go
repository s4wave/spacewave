//go:build js && !bldr_indexeddb

package browser_storage

import (
	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	"github.com/pkg/errors"
)

// OpfsStorage implements OPFS-backed browser storage.
// Currently a stub that returns "not implemented" errors.
// Will be replaced by the OPFS volume implementation.
type OpfsStorage struct {
	prefix string
}

// NewOpfsStorage constructs an OPFS storage handle.
func NewOpfsStorage(prefix string) storage.Storage {
	return &OpfsStorage{prefix: prefix}
}

// GetStorageInfo returns StorageInfo.
func (s *OpfsStorage) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{}
}

// AddFactories adds the factories to the resolver.
func (s *OpfsStorage) AddFactories(b bus.Bus, sr *static.Resolver) {
	// No factories yet: OPFS volume not implemented.
}

// BuildVolumeConfig creates the volume config for the store ID.
func (s *OpfsStorage) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	return nil, errors.New("OPFS volume storage not yet implemented")
}

// DeleteVolume removes the volume.
func (s *OpfsStorage) DeleteVolume(id string) error {
	return errors.New("OPFS volume storage not yet implemented")
}

func init() {
	storageMethods = append(storageMethods, func(b bus.Bus, prefix string) []storage.Storage {
		return []storage.Storage{NewOpfsStorage(prefix)}
	})
}

// _ is a type assertion
var _ storage.Storage = ((*OpfsStorage)(nil))
