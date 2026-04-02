//go:build js && !bldr_indexeddb

package browser_storage

import (
	"strings"

	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	sqlite_wasm "github.com/aperturerobotics/hydra/sql/sqlite-wasm"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	volume_sqlite "github.com/aperturerobotics/hydra/volume/sqlite"
)

// SqliteOPFS implements SQLite-backed browser storage via OPFS.
type SqliteOPFS struct {
	prefix string
}

// NewSqliteOPFS constructs a SQLite OPFS storage handle.
func NewSqliteOPFS(prefix string) storage.Storage {
	prefix = strings.TrimSpace(prefix)
	if len(prefix) != 0 && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &SqliteOPFS{prefix: prefix}
}

// GetStorageInfo returns StorageInfo.
func (s *SqliteOPFS) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{}
}

// AddFactories adds the factories to the resolver.
func (s *SqliteOPFS) AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_sqlite.NewFactory(b))
}

// BuildVolumeConfig creates the volume config for the store ID.
// The path is a virtual database name within the OPFS sahpool VFS.
func (s *SqliteOPFS) BuildVolumeConfig(id string, baseVolCtrlConf *volume_controller.Config) (config.Config, error) {
	return &volume_sqlite.Config{
		Path:         "/" + s.prefix + id + ".db",
		Table:        "hydra",
		VolumeConfig: baseVolCtrlConf,
	}, nil
}

// DeleteVolume removes the SQLite database from OPFS via the sqlite-wasm RPC client.
func (s *SqliteOPFS) DeleteVolume(id string) error {
	return sqlite_wasm.DeleteDatabase("/" + s.prefix + id + ".db")
}

func init() {
	storageMethods = append(storageMethods, func(b bus.Bus, prefix string) []storage.Storage {
		return []storage.Storage{NewSqliteOPFS(prefix)}
	})
}

// _ is a type assertion
var _ storage.Storage = ((*SqliteOPFS)(nil))
