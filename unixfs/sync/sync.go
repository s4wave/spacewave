package unixfs_sync

import (
	"context"
	"io/fs"
	"os"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
	"github.com/go-git/go-billy/v5/osfs"
)

// Sync recursively synchronizes the contents of the UnixFS to disk.
//
// Attempts to skip files by checking size and modification time.
// The output path does not have to be empty when starting.
// filterCb is optional and is called with each element in the fs tree.
// TODO: Does not (yet) support symlinks or other non-file and non-dir node types.
func Sync(
	ctx context.Context,
	outPath string,
	fsHandle *unixfs.FSHandle,
	deleteMode DeleteMode,
	filterCb FilterCb,
) error {
	if fsHandle.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// create / reset outPath
	if _, err := os.Stat(outPath); err == nil {
		if err := os.RemoveAll(outPath); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(outPath, 0755); err != nil {
		return err
	}

	// construct a BillyFS at the outPath & checkout
	outFS := osfs.New(outPath)
	return SyncToBilly(ctx, outFS, fsHandle, deleteMode, filterCb)
}

// SyncFromDisk syncs from disk to the given UnixFS FSHandle.
func SyncFromDisk(
	ctx context.Context,
	destHandle *unixfs.FSHandle,
	srcPath string,
	deleteMode DeleteMode,
	filterCb FilterCb,
) error {
	return SyncFromFS(ctx, destHandle, os.DirFS(srcPath), deleteMode, filterCb)
}

// SyncFromFS syncs from the given fs.FS to the UnixFS FSHandle.
func SyncFromFS(
	ctx context.Context,
	destHandle *unixfs.FSHandle,
	srcFs fs.FS,
	deleteMode DeleteMode,
	filterCb FilterCb,
) error {
	srcCursor, err := unixfs_iofs.NewFSCursor(srcFs)
	if err != nil {
		return err
	}
	defer srcCursor.Release()

	diskFS := unixfs.NewFS(ctx, nil, srcCursor, nil)
	defer diskFS.Release()

	diskRef, err := diskFS.AddRootReference(ctx)
	if err != nil {
		return err
	}
	defer diskRef.Release()

	return SyncToUnixfs(ctx, destHandle, diskRef, deleteMode, filterCb)
}
