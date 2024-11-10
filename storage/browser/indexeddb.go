//go:build js
// +build js

package browser_storage

import (
	"strings"

	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	volume_indexeddb "github.com/aperturerobotics/hydra/volume/js/indexeddb"
)

// IndexedDB implements the indexeddb-backed storage.
type IndexedDB struct {
	prefix  string
	verbose bool
}

// NewIndexedDB constructs an IndexedDB storage handle.
func NewIndexedDB(prefix string, verbose bool) storage.Storage {
	prefix = strings.TrimSpace(prefix)
	if len(prefix) != 0 && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &IndexedDB{prefix: prefix, verbose: verbose}
}

// GetStorageInfo returns StorageInfo.
func (i *IndexedDB) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{}
}

// AddFactories adds the factories to the resolver.
func (i *IndexedDB) AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_indexeddb.NewFactory(b))
}

// BuildVolumeConfig creates the volume config for the store ID.
// Returns nil if the storage cannot produce Volume.
func (i *IndexedDB) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	return &volume_indexeddb.Config{
		DatabaseName: i.prefix + id,
		Verbose:      i.verbose,
		VolumeConfig: baseVolCtrlConf,
	}, nil
}

func init() {
	storageMethods = append(storageMethods, func(b bus.Bus, prefix string) []storage.Storage {
		return []storage.Storage{NewIndexedDB(prefix, false)}
	})
}

// _ is a type assertion
var _ storage.Storage = ((*IndexedDB)(nil))
