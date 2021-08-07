package electron_storage

import (
	"path"

	"github.com/aperturerobotics/bldr/runtime"
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
//
// if rootDir is empty, uses ./data
func NewElectronBoltDB(verbose bool, rootDir string) runtime.Storage {
	if rootDir == "" {
		rootDir = "./data"
	}
	return &ElectronBoltDB{verbose: verbose, rootDir: rootDir}
}

// GetStorageInfo returns StorageInfo.
func (i *ElectronBoltDB) GetStorageInfo() *runtime.StorageInfo {
	return &runtime.StorageInfo{
		Isolated: true,
		Cache:    false,
	}
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
	storageMethods = append(storageMethods, func(b bus.Bus, sr *static.Resolver) []runtime.Storage {
		sr.AddFactory(volume_bolt.NewFactory(b))
		return []runtime.Storage{NewElectronBoltDB(false, "")}
	})
}

// _ is a type assertion
var _ runtime.Storage = ((*ElectronBoltDB)(nil))
