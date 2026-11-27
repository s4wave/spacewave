// Package storage_node binds Node SQLite connections to the existing volume factory.
package storage_node

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/storage"
	sqlite_rpc "github.com/s4wave/spacewave/db/sql/sqlite-rpc"
	sqlite_bridge "github.com/s4wave/spacewave/db/sql/sqlite-wasm/rpc"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_sqlite "github.com/s4wave/spacewave/db/volume/sqlite"
	"github.com/sirupsen/logrus"
)

// Storage uses the instance's SQLite bridge under an exclusively owned directory.
// The Node Worker acquires and retains directory ownership before constructing it.
type Storage struct {
	// client owns physical SQLite connections in the engine Worker.
	client sqlite_bridge.SRPCSqliteBridgeClient
	// directory is the canonical directory owned by the Worker.
	directory string
}

// NewStorage binds a bridge to a directory whose writer lock is already held.
func NewStorage(client sqlite_bridge.SRPCSqliteBridgeClient, directory string) *Storage {
	return &Storage{client: client, directory: directory}
}

// GetStorageInfo returns the persistent storage capability.
func (s *Storage) GetStorageInfo() *storage.StorageInfo {
	return &storage.StorageInfo{}
}

// AddFactories binds SQLite volume creation to this instance's bridge.
func (s *Storage) AddFactories(b bus.Bus, resolver *static.Resolver) {
	resolver.AddFactory(volume_sqlite.NewFactoryWithOpener(b, s.open))
}

// BuildVolumeConfig selects an existing SQLite volume within the owned
// directory.
func (s *Storage) BuildVolumeConfig(id string, base *volume_controller.Config) (config.Config, error) {
	path, err := s.volumePath(id)
	if err != nil {
		return nil, err
	}
	return &volume_sqlite.Config{Path: path, Table: "bldr", VolumeConfig: base}, nil
}

// DeleteVolume deletes an already-closed volume through its owning bridge.
func (s *Storage) DeleteVolume(id string) error {
	path, err := s.volumePath(id)
	if err != nil {
		return err
	}
	_, err = s.client.DeleteDb(context.Background(), &sqlite_bridge.DeleteDbRequest{Path: path})
	return err
}

// open constructs the standard volume from an instance-bound SQL pool.
func (s *Storage) open(ctx context.Context, le *logrus.Entry, conf *volume_sqlite.Config) (volume.Volume, error) {
	store, err := sqlite_rpc.OpenStore(ctx, s.client, conf.GetPath(), conf.GetTable())
	if err != nil {
		return nil, err
	}
	return volume_sqlite.NewWithStore(ctx, le, conf, store, func() error {
		_, err := s.client.DeleteDb(context.Background(), &sqlite_bridge.DeleteDbRequest{Path: conf.GetPath()})
		return err
	})
}

// volumePath accepts one stable file component without collapsing distinct IDs.
func (s *Storage) volumePath(id string) (string, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\\x00") {
		return "", errors.New("invalid Node volume ID")
	}
	return filepath.Join(s.directory, id+".db"), nil
}

// _ is a type assertion
var _ storage.Storage = (*Storage)(nil)
