// +build js

package browser_storage

import (
	"github.com/aperturerobotics/bldr/runtime"
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
func NewIndexedDB(verbose bool) runtime.Storage {
	return &IndexedDB{verbose: verbose}
}

// GetStorageInfo returns StorageInfo.
func (i *IndexedDB) GetStorageInfo() *runtime.StorageInfo {
	return &runtime.StorageInfo{
		Isolated: true,
		Cache:    false,
	}
}

// BuildVolumeConfig creates the volume config for the store ID.
// Returns nil if the storage cannot produce Volume.
func (i *IndexedDB) BuildVolumeConfig(id string) config.Config {
	return &volume_indexeddb.Config{
		DatabaseName: id,
		Verbose:      i.verbose,
	}
}

func init() {
	storageMethods = append(storageMethods, func(b bus.Bus, sr *static.Resolver) []runtime.Storage {
		sr.AddFactory(volume_indexeddb.NewFactory(b))
		return []runtime.Storage{NewIndexedDB(false)}
	})
}

// _ is a type assertion
var _ runtime.Storage = ((*IndexedDB)(nil))
