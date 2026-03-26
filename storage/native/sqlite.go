//go:build !js && bldr_sqlite

package storage_native

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	volume_sqlite "github.com/aperturerobotics/hydra/volume/sqlite"
	"github.com/pkg/errors"
)

const SqliteDBExt = ".db"

// SqliteDB implements the SqliteDB database.
type SqliteDB struct {
	verbose bool
	rootDir string
}

// NewSqliteDB constructs an SqliteDB storage handle.
func NewSqliteDB(verbose bool, rootDir string) storage.Storage {
	return &SqliteDB{verbose: verbose, rootDir: rootDir}
}

// GetStorageInfo returns StorageInfo.
func (i *SqliteDB) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{}
}

// AddFactories adds the factories to the resolver.
func (i *SqliteDB) AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_sqlite.NewFactory(b))
}

// BuildVolumeConfig creates the volume config for the store ID.
// Returns nil if the storage cannot produce Volume.
func (i *SqliteDB) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	// replace any slashes with underscores
	filename := strings.ReplaceAll(id, "/", "_") + SqliteDBExt
	if cleanFilename := filepath.Clean(filename); cleanFilename != filename {
		return nil, errors.Errorf("invalid storage id: %s", filename)
	}

	return &volume_sqlite.Config{
		Path:         filepath.Join(i.rootDir, filename),
		Table:        "bldr",
		Verbose:      i.verbose,
		VolumeConfig: baseVolCtrlConf,
	}, nil
}

// DeleteVolume removes the SQLite database file for the given volume ID.
func (i *SqliteDB) DeleteVolume(id string) error {
	filename := strings.ReplaceAll(id, "/", "_") + SqliteDBExt
	return os.Remove(filepath.Join(i.rootDir, filename))
}

func init() {
	storageMethods = append(storageMethods, func(b bus.Bus, rootDir string) []storage.Storage {
		return []storage.Storage{NewSqliteDB(false, rootDir)}
	})
}

// _ is a type assertion
var _ storage.Storage = ((*SqliteDB)(nil))
