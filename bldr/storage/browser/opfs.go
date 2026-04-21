//go:build js && !bldr_indexeddb

package browser_storage

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/s4wave/spacewave/bldr/storage"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/unixfs"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_opfs "github.com/s4wave/spacewave/db/volume/js/opfs"
)

// OpfsStorage implements OPFS-backed browser storage.
type OpfsStorage struct {
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
		RootPath:               rootPath,
		BlockCompactionTrigger: 8,
		VolumeConfig:           baseVolCtrlConf,
	}, nil
}

// DeleteVolume removes the OPFS directory for the given volume ID.
func (s *OpfsStorage) DeleteVolume(id string) error {
	rootPath := s.prefix + id
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}
	parts, _ := unixfs.SplitPath(rootPath)
	parent := root
	for _, p := range parts[:len(parts)-1] {
		parent, err = opfs.GetDirectory(parent, p, false)
		if err != nil {
			if opfs.IsNotFound(err) {
				return nil
			}
			return err
		}
	}
	err = opfs.DeleteEntry(parent, parts[len(parts)-1], true)
	if err != nil && !opfs.IsNotFound(err) {
		return err
	}
	return nil
}

func init() {
	storageMethods = append(storageMethods, func(b bus.Bus, prefix string) []storage.Storage {
		return []storage.Storage{NewOpfsStorage(prefix)}
	})
}

// _ is a type assertion.
var _ storage.Storage = ((*OpfsStorage)(nil))
