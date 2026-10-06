//go:build !js && !wasip1

// Package volume_s4db implements the Volume on one s4db database file.
//
// Blocks, metadata, and the garbage collection graph share the file's key
// space. The database commits each write transaction atomically and durably,
// so a publication writes its blocks and references in one commit.
package volume_s4db

import (
	"context"
	"errors"
	"os"

	"github.com/aperturerobotics/controllerbus/controller"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	coord_s4db "github.com/s4wave/spacewave/db/coord/s4db"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_vlogger "github.com/s4wave/spacewave/db/kvtx/vlogger"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the s4db volume controller.
const ControllerID = "hydra/volume/s4db"

// Version is the version of the s4db volume implementation.
var Version = controller.MustParseVersion("0.0.1")

// Volume is the s4db volume.
type Volume = volume_kvtx.Volume

// NewVolume opens the database file named by conf, creating it when absent.
func NewVolume(ctx context.Context, le *logrus.Entry, conf *Config) (*Volume, error) {
	// Check the config and open the database.
	if err := conf.Validate(); err != nil {
		return nil, volume.Permanent(err)
	}
	keys, err := store_kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}
	path := conf.GetPath()
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		return nil, err
	}
	var store kvtx.Store = db
	if conf.GetVerbose() {
		store = kvtx_vlogger.NewVLogger(le, store)
	}

	// Build the Volume.
	stats := func(ctx context.Context) (*volume.StorageStats, error) {
		return storageStats(ctx, db, path)
	}
	vol, err := volume_kvtx.NewVolume(
		ctx, ControllerID, keys, store, conf.GetStoreConfig(),
		conf.GetNoGenerateKey(), conf.GetNoWriteKey(), stats, db.Close,
		func() error { return os.Remove(path) },
	)
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}

	// Coordinate writers across processes through the database file.
	vol.Coordinator = coord_s4db.NewCoordinator(db, coord_inmem.ForVolume(vol.GetID()))
	return vol, nil
}

// storageStats reports the size of the file at path and the number of keys
// in db.
func storageStats(ctx context.Context, db *s4db.DB, path string) (*volume.StorageStats, error) {
	// Measure the file.
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// Count the keys in a snapshot.
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	count, err := tx.Size(ctx)
	if err != nil {
		return nil, err
	}
	return &volume.StorageStats{TotalBytes: uint64(fi.Size()), BlockCount: count}, nil //nolint:gosec
}
