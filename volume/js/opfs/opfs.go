//go:build js

package volume_opfs

import (
	"context"

	block_store_opfs "github.com/aperturerobotics/hydra/block/store/opfs"
	"github.com/aperturerobotics/hydra/opfs"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	skvtx "github.com/aperturerobotics/hydra/store/kvtx"
	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
	store_objstore_opfs "github.com/aperturerobotics/hydra/store/objstore/opfs"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/hydra/volume"
	kvtx "github.com/aperturerobotics/hydra/volume/common/kvtx"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID identifies the OPFS volume controller.
const ControllerID = "hydra/volume/opfs"

// Version is the version of the OPFS volume implementation.
var Version = semver.MustParse("0.0.1")

// Opfs implements an OPFS-backed volume.
type Opfs = kvtx.Volume

// NewOpfs builds a new OPFS volume, opening or creating the directory tree.
func NewOpfs(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
) (*Opfs, error) {
	kk, err := kvkey.NewKVKey(conf.GetKvKeyOpts())
	if err != nil {
		return nil, err
	}

	rootPath := conf.GetRootPath()
	lockPrefix := conf.GetLockPrefix()
	if lockPrefix == "" {
		lockPrefix = rootPath
	}

	// Open or create the OPFS directory for this volume.
	opfsRoot, err := opfs.GetRoot()
	if err != nil {
		return nil, errors.Wrap(err, "opfs GetRoot")
	}

	pathParts, _ := unixfs.SplitPath(rootPath)
	volDir, err := opfs.GetDirectoryPath(opfsRoot, pathParts, true)
	if err != nil {
		return nil, errors.Wrap(err, "create volume directory")
	}

	// Create the blocks/ subdirectory for the per-file block store.
	blocksDir, err := opfs.GetDirectory(volDir, "blocks", true)
	if err != nil {
		return nil, errors.Wrap(err, "create blocks directory")
	}

	// Per-file block store: no transaction-level WebLock, just per-file locks.
	blkStore := block_store_opfs.NewBlockStore(
		blocksDir,
		lockPrefix+"/blocks",
		conf.GetStoreConfig().GetHashType(),
	)

	// Object store: per-file write locking with readers-writer WebLock for ACID.
	objStore := store_objstore_opfs.NewStore(
		volDir,
		lockPrefix+"|objstore",
		lockPrefix+"/obj",
	)

	var store skvtx.Store = objStore
	if conf.GetVerbose() {
		store = kvtx_vlogger.NewVLogger(le, store)
	}

	statsFn := func(ctx context.Context) (*volume.StorageStats, error) {
		tx, txErr := objStore.NewTransaction(ctx, false)
		if txErr != nil {
			return nil, txErr
		}
		defer tx.Discard()
		count, txErr := tx.Size(ctx)
		if txErr != nil {
			return nil, txErr
		}
		return &volume.StorageStats{
			BlockCount: count,
		}, nil
	}

	return kvtx.NewVolumeWithBlockStore(
		ctx,
		ControllerID,
		kk,
		store,
		blkStore,
		conf.GetStoreConfig(),
		conf.GetNoGenerateKey(),
		conf.GetNoWriteKey(),
		statsFn,
		nil, // close: no-op (OPFS has no handles to release)
		func() error {
			// Delete: navigate to the parent, then remove the leaf directory.
			parts, _ := unixfs.SplitPath(rootPath)
			parent := opfsRoot
			for _, p := range parts[:len(parts)-1] {
				var err error
				parent, err = opfs.GetDirectory(parent, p, false)
				if err != nil {
					if opfs.IsNotFound(err) {
						return nil
					}
					return err
				}
			}
			return opfs.DeleteEntry(parent, parts[len(parts)-1], true)
		},
	)
}
