//go:build !js && !wasip1

package volume_bolt

import (
	"context"
	"os"
	"path/filepath"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	coord_bolt "github.com/s4wave/spacewave/db/coord/bolt"
	coord_filelock "github.com/s4wave/spacewave/db/coord/filelock"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	skvtx "github.com/s4wave/spacewave/db/store/kvtx"
	sbolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	"github.com/s4wave/spacewave/db/volume"
	kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the Bolt volume controller.
const ControllerID = "hydra/volume/bolt"

// Version is the version of the bolt implementation.
var Version = controller.MustParseVersion("0.0.1")

// Bolt implements a BoltDB backed volume.
type Bolt = kvtx.Volume

// NewBolt builds a new Bolt volume, opening the database.
func NewBolt(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
) (*Bolt, error) {
	// Build the key encoder.
	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	// Open the database, waiting for the file lock without a deadline. Keep
	// the default synced freelist: bbolt multi-process access is unsafe once
	// another process opens a DB whose freelist lives only in memory.
	bdbOpts := &bdb.Options{
		FreelistType: bdb.FreelistMapType,
		Exclusive:    conf.GetExclusive(),
	}
	store, err := sbolt.Open(conf.GetPath(), 0o644, bdbOpts, []byte("hydra"))
	if err != nil {
		return nil, err
	}

	// Wrap the store with the configured batching and logging.
	var vstore skvtx.Store = store
	var batchStore *sbolt.BatchStore
	if batchSize := conf.GetBatchSize(); batchSize > 1 {
		batchStore = sbolt.NewBatchStore(store, int(batchSize))
		vstore = batchStore
	}
	if conf.GetVerbose() {
		vstore = kvtx_vlogger.NewVLogger(le, vstore)
	}

	// Flush pending batched writes before closing the database.
	closeFn := store.Close
	if batchStore != nil {
		closeFn = func() error {
			if err := batchStore.Flush(); err != nil {
				return err
			}
			return store.Close()
		}
	}

	// Build the volume, reporting the file size and block count as its
	// storage stats.
	boltDB := store.GetDB()
	path := conf.GetPath()
	vol, err := kvtx.NewVolume(
		ctx,
		ControllerID,
		kvkey,
		vstore,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		conf.GetNoWriteKey(),
		func(ctx context.Context) (*volume.StorageStats, error) {
			var totalBytes uint64
			if fi, err := os.Stat(boltDB.Path()); err == nil {
				if fi.Size() < 0 {
					return nil, errors.New("bolt file size is negative")
				}
				totalBytes = uint64(fi.Size()) //nolint:gosec
			}
			tx, err := store.NewTransaction(ctx, false)
			if err != nil {
				return nil, err
			}
			defer tx.Discard()
			count, err := tx.Size(ctx)
			if err != nil {
				return nil, err
			}
			return &volume.StorageStats{
				TotalBytes: totalBytes,
				BlockCount: count,
			}, nil
		},
		closeFn,
		func() error { return os.Remove(path) },
	)
	if err != nil {
		return nil, err
	}

	// Coordinate writers across processes through the volume file lock.
	vol.Coordinator = coord_filelock.NewCoordinator(
		filepath.Dir(path),
		path,
		coord_bolt.NewCoordinator(boltDB, coord_inmem.ForVolume(vol.GetID())),
	)
	return vol, nil
}

// boltDBProvider is implemented by types that expose a *bdb.DB.
type boltDBProvider interface {
	GetDB() *bdb.DB
}

// storeUnwrapper is implemented by store wrappers like VLoggerStore.
type storeUnwrapper interface {
	Unwrap() skvtx.Store
}

// GetBoltDB extracts the *bdb.DB from a Volume if it is bolt-backed.
// Returns nil if the volume does not use bolt. Handles wrapped stores
// (VLoggerStore, BatchStore).
func GetBoltDB(vol volume.Volume) *bdb.DB {
	kv, ok := vol.(kvtx.KvtxVolume)
	if !ok {
		return nil
	}
	var store any = kv.GetKvtxStore()
	for range 10 {
		if p, ok := store.(boltDBProvider); ok {
			return p.GetDB()
		}
		if u, ok := store.(storeUnwrapper); ok {
			store = u.Unwrap()
			continue
		}
		break
	}
	return nil
}

// _ is a type assertion
var (
	_ volume.Volume   = (*Bolt)(nil)
	_ kvtx.KvtxVolume = (*Bolt)(nil)
)
