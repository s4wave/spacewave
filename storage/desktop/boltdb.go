package electron_storage

import (
	"path"

	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_bolt "github.com/aperturerobotics/hydra/volume/bolt"
)

const ElectronBoltDBExt = ".bdb"

// ElectronBoltDB implements the ElectronBoltDB database in the Electron data dir.
type ElectronBoltDB struct {
	verbose bool
	rootDir string
}

// NewElectronBoltDB constructs an ElectronBoltDB storage handle.
func NewElectronBoltDB(verbose bool, rootDir string) storage.Storage {
	return &ElectronBoltDB{verbose: verbose, rootDir: rootDir}
}

// GetStorageInfo returns StorageInfo.
func (i *ElectronBoltDB) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{
		Isolated: true,
		Cache:    false,
	}
}

// AddFactories adds the factories to the resolver.
func (i *ElectronBoltDB) AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_bolt.NewFactory(b))
}

// BuildVolumeConfig creates the volume config for the store ID.
// Returns nil if the storage cannot produce Volume.
func (i *ElectronBoltDB) BuildVolumeConfig(id string) config.Config {
	return &volume_bolt.Config{
		Path:    path.Join(i.rootDir, id+ElectronBoltDBExt),
		Verbose: i.verbose,
	}
}

func init() {
	storageMethods = append(storageMethods, func(b bus.Bus, rootDir string) []storage.Storage {
		return []storage.Storage{NewElectronBoltDB(false, rootDir)}
	})
}

// _ is a type assertion
var _ storage.Storage = ((*ElectronBoltDB)(nil))
