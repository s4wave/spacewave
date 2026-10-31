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

// ErrMigrateShared is returned when a Volume file needs a format migration
// while another process has it open.
var ErrMigrateShared = errors.New("volume format migration waits for other processes to close the file")

// Volume is the s4db volume.
type Volume = volume_kvtx.Volume

// NewVolume opens the database file named by conf, creating it when absent
// and converting a bolt Volume file in place.
func NewVolume(ctx context.Context, le *logrus.Entry, conf *Config) (*Volume, error) {
	// Check the config.
	if err := conf.Validate(); err != nil {
		return nil, volume.Permanent(err)
	}
	keys, err := store_kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	// Convert a bolt Volume file.
	path := conf.GetPath()
	if err := convertBolt(ctx, le, path); err != nil {
		return nil, err
	}

	// Open the database, counting the values of each of the Volume's key
	// families.
	families := volume_kvtx.ValueFamilies(keys)
	prefixes := make([][]byte, len(families))
	for i, f := range families {
		prefixes[i] = f.Prefix
	}
	opts := s4db.Options{
		CacheBytes: int(conf.GetCacheBytes()),
		Families:   prefixes,
	}

	// Open it, converting a file the previous engine wrote in place first.
	db, err := s4db.Open(path, opts)
	if errors.Is(err, s4db.ErrPreviousFormat) {
		if err := convertPrevious(ctx, le, path); err != nil {
			return nil, err
		}
		db, err = s4db.Open(path, opts)
	}
	if err != nil {
		return nil, err
	}
	var store kvtx.Store = db
	if conf.GetVerbose() {
		store = kvtx_vlogger.NewVLogger(le, store)
	}

	// A migration rewrites keys that another process may still read in the
	// old layout, so it waits until this process holds the file alone.
	stale, err := volume_kvtx.NeedsFormatUpgrade(ctx, db, keys)
	if err == nil && stale {
		var shared bool
		shared, err = db.Shared()
		if err == nil && shared {
			err = ErrMigrateShared
		}
	}
	if err != nil {
		return nil, errors.Join(err, db.Close())
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

	// Coordinate writers across processes through the database file, log
	// puts without references, measure the file and its disk for the volume's
	// health, and report the values each key family wrote and freed and the
	// longest a commit waited behind a background step. The database returns
	// what collection frees on its own, so the reserve state needs no release.
	vol.Coordinator = coord_s4db.NewCoordinator(db, coord_inmem.ForVolume(vol.GetID()))
	vol.SetLogger(le)
	vol.SetStorageFlows(func(f *volume.VolumeFlows) {
		st, err := db.Stats(context.Background())
		if err != nil {
			return
		}
		f.Values = valueFlows(families, st.Families)
		f.BackgroundWaitMax = st.BackgroundWaitMax
	})
	vol.SetHealthSizes(func(ctx context.Context) (volume_kvtx.HealthSizes, error) {
		return healthSizes(ctx, db, path, conf.GetReserveBytes())
	})
	return vol, nil
}

// valueFlows names the flows of the database opened with families, the last
// of which counts the keys under none.
func valueFlows(families []volume_kvtx.ValueFamily, flows []s4db.Flow) []volume.ValueFlow {
	out := make([]volume.ValueFlow, len(flows))
	for i, f := range flows {
		name := "other"
		if i < len(families) {
			name = families[i].Name
		}
		out[i] = volume.ValueFlow{Family: name, Written: uint64(f.Written), Freed: uint64(f.Freed)} // #nosec G115 -- a flow is not negative.
	}
	return out
}

// GetDB returns the database of an s4db Volume, or nil for another Volume.
func GetDB(vol volume.Volume) *s4db.DB {
	// Unwrap the verbose logger from the Volume's store.
	kv, ok := vol.(volume_kvtx.KvtxVolume)
	if !ok {
		return nil
	}
	store := kv.GetKvtxStore()
	if l, ok := store.(*kvtx_vlogger.VLoggerStore); ok {
		store = l.Unwrap()
	}
	db, _ := store.(*s4db.DB)
	return db
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

// healthSizes measures the file's length and live values through db and the
// space free on its disk, and chooses the disk reserve: reserveBytes when
// positive, none when negative, and otherwise the disk one collection needs
// before it frees any.
func healthSizes(ctx context.Context, db *s4db.DB, path string, reserveBytes int64) (volume_kvtx.HealthSizes, error) {
	// Read the file's sizes as of the published state.
	st, err := db.Stats(ctx)
	if err != nil {
		return volume_kvtx.HealthSizes{}, err
	}

	// Measure the free disk.
	freeBytes, err := freeDiskBytes(path)
	if err != nil {
		return volume_kvtx.HealthSizes{}, err
	}

	// Choose the reserve.
	if reserveBytes == 0 {
		reserveBytes = st.CollectBytes
	}
	return volume_kvtx.HealthSizes{
		FileBytes:      uint64(st.FileBytes), //nolint:gosec
		LiveValueBytes: uint64(st.LiveBytes), //nolint:gosec
		FreeDiskBytes:  freeBytes,
		ReserveBytes:   uint64(max(reserveBytes, 0)), //nolint:gosec
	}, nil
}
