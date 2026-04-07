//go:build !js && !wasip1

package volume_bolt

import (
	"context"
	"os"

	bdb "github.com/aperturerobotics/bbolt"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	skvtx "github.com/aperturerobotics/hydra/store/kvtx"
	sbolt "github.com/aperturerobotics/hydra/store/kvtx/bolt"
	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/volume"
	kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the Bolt volume controller.
const ControllerID = "hydra/volume/bolt"

// Version is the version of the bolt implementation.
var Version = semver.MustParse("0.0.1")

// Bolt implements a BoltDB backed volume.
type Bolt = kvtx.Volume

// NewBolt builds a new Bolt volume, opening the database.
func NewBolt(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
) (*Bolt, error) {
	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	// set defaults for performance with single writer on db
	bdbOpts := &bdb.Options{
		Timeout:        0,
		NoFreelistSync: !conf.GetFreelistSync(),
		NoGrowSync:     false,
		FreelistType:   bdb.FreelistMapType,
		NoSync:         !conf.GetSync(),
	}

	store, err := sbolt.Open(
		conf.GetPath(),
		0o644,
		bdbOpts,
		[]byte("hydra"),
	)
	if err != nil {
		return nil, err
	}

	var vstore skvtx.Store = store
	var batchStore *sbolt.BatchStore
	if batchSize := conf.GetBatchSize(); batchSize > 1 {
		batchStore = sbolt.NewBatchStore(store, int(batchSize))
		vstore = batchStore
	}
	if conf.GetVerbose() {
		vstore = kvtx_vlogger.NewVLogger(le, vstore)
	}

	closeFn := store.GetDB().Close
	if batchStore != nil {
		origClose := closeFn
		closeFn = func() error {
			if err := batchStore.Flush(); err != nil {
				return err
			}
			return origClose()
		}
	}

	boltDB := store.GetDB()
	path := conf.GetPath()
	return kvtx.NewVolume(
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
				totalBytes = uint64(fi.Size())
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
	var store interface{} = kv.GetKvtxStore()
	for i := 0; i < 10; i++ {
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
	_ volume.Volume   = ((*Bolt)(nil))
	_ kvtx.KvtxVolume = ((*Bolt)(nil))
)
