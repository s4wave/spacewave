//go:build !js

package storage_native

import (
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_bolt "github.com/aperturerobotics/hydra/volume/bolt"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	"github.com/pkg/errors"
)

const BoltDBExt = ".bdb"

// BoltDB implements the BoltDB database in the Electron data dir.
type BoltDB struct {
	verbose bool
	rootDir string
}

// NewBoltDB constructs an BoltDB storage handle.
func NewBoltDB(verbose bool, rootDir string) storage.Storage {
	return &BoltDB{verbose: verbose, rootDir: rootDir}
}

// GetStorageInfo returns StorageInfo.
func (i *BoltDB) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{}
}

// AddFactories adds the factories to the resolver.
func (i *BoltDB) AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_bolt.NewFactory(b))
}

// BuildVolumeConfig creates the volume config for the store ID.
// Returns nil if the storage cannot produce Volume.
func (i *BoltDB) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	// replace any slashes with underscores
	filename := strings.ReplaceAll(id, "/", "_") + BoltDBExt
	if cleanFilename := filepath.Clean(filename); cleanFilename != filename {
		return nil, errors.Errorf("invalid storage id: %s", filename)
	}

	return &volume_bolt.Config{
		Path:         filepath.Join(i.rootDir, filename),
		Verbose:      i.verbose,
		Sync:         true,
		VolumeConfig: baseVolCtrlConf,
	}, nil
}

func init() {
	storageMethods = append(storageMethods, func(b bus.Bus, rootDir string) []storage.Storage {
		return []storage.Storage{NewBoltDB(false, rootDir)}
	})
}

// _ is a type assertion
var _ storage.Storage = ((*BoltDB)(nil))
