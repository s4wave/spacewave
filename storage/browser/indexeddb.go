//go:build js
// +build js

package browser_storage

import (
	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_indexeddb "github.com/aperturerobotics/hydra/volume/js/indexeddb"
)

// IndexedDB implements the indexeddb-backed storage.
type IndexedDB struct {
	verbose bool
}

// NewIndexedDB constructs an IndexedDB storage handle.
func NewIndexedDB(verbose bool) storage.Storage {
	return &IndexedDB{verbose: verbose}
}

// GetStorageInfo returns StorageInfo.
func (i *IndexedDB) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{
		Isolated: true,
		Cache:    false,
	}
}

// AddFactories adds the factories to the resolver.
func (i *IndexedDB) AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_indexeddb.NewFactory(b))
}

// BuildVolumeConfig creates the volume config for the store ID.
// Returns nil if the storage cannot produce Volume.
func (i *IndexedDB) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) config.Config {
	return &volume_indexeddb.Config{
		DatabaseName: id,
		Verbose:      i.verbose,
		VolumeConfig: baseVolCtrlConf,
	}
}

func init() {
	storageMethods = append(storageMethods, func(b bus.Bus) []storage.Storage {
		return []storage.Storage{NewIndexedDB(false)}
	})
}

// _ is a type assertion
var _ storage.Storage = ((*IndexedDB)(nil))
