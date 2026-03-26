//go:build js

package volume_indexeddb

import (
	"context"

	"github.com/aperturerobotics/go-indexeddb/idb"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	skvtx "github.com/aperturerobotics/hydra/store/kvtx"
	sindexeddb "github.com/aperturerobotics/hydra/store/kvtx/js/indexeddb"
	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/volume"
	kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the IndexedDB volume controller.
const ControllerID = "hydra/volume/indexeddb"

// Version is the version of the indexeddb implementation.
var Version = semver.MustParse("0.0.1")

// IndexedDB implements a IndexedDB backed volume.
type IndexedDB = kvtx.Volume

// NewIndexedDB builds a new IndexedDB volume, opening the database.
func NewIndexedDB(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
) (*IndexedDB, error) {
	kvkey, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	storeName := conf.GetStoreName()
	if storeName == "" {
		storeName = "hydra"
	}

	istore, err := sindexeddb.Open(
		ctx,
		conf.GetDatabaseName(),
		storeName,
	)
	if err != nil {
		return nil, err
	}

	var store skvtx.Store = istore
	if conf.GetVerbose() {
		store = kvtx_vlogger.NewVLogger(le, store)
	}

	statsFn := func(ctx context.Context) (*volume.StorageStats, error) {
		totalBytes, err := istore.GetStorageTally(ctx)
		if err != nil {
			return nil, err
		}
		tx, err := istore.NewTransaction(ctx, false)
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
	}

	dbName := conf.GetDatabaseName()
	return kvtx.NewVolume(
		ctx,
		ControllerID,
		kvkey,
		store,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		conf.GetNoWriteKey(),
		statsFn,
		istore.Close,
		func() error {
			req, err := idb.Global().DeleteDatabase(dbName)
			if err != nil {
				return err
			}
			return req.Await(ctx)
		},
	)
}
