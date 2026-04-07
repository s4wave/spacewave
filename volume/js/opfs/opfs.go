//go:build js

package volume_opfs

import (
	"context"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/unixfs"
	kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	skvtx "github.com/aperturerobotics/hydra/store/kvtx"
	sopfs "github.com/aperturerobotics/hydra/store/kvtx/js/opfs"
	kvtx_vlogger "github.com/aperturerobotics/hydra/store/kvtx/vlogger"
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

	// Create the kvtx.Store backed by OPFS.
	lockName := lockPrefix + "|kvtx"
	ostore := sopfs.NewStore(volDir, lockName)

	var store skvtx.Store = ostore
	if conf.GetVerbose() {
		store = kvtx_vlogger.NewVLogger(le, store)
	}

	statsFn := func(ctx context.Context) (*volume.StorageStats, error) {
		tx, err := ostore.NewTransaction(ctx, false)
		if err != nil {
			return nil, err
		}
		defer tx.Discard()
		count, err := tx.Size(ctx)
		if err != nil {
			return nil, err
		}
		return &volume.StorageStats{
			BlockCount: count,
		}, nil
	}

	return kvtx.NewVolume(
		ctx,
		ControllerID,
		kk,
		store,
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
