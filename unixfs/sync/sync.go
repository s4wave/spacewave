package unixfs_sync

import (
	"context"
	"os"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/go-git/go-billy/v5/osfs"
)

// Sync recursively synchronizes the contents of the UnixFS to disk.
//
// Attempts to skip files by checking size and modification time.
// The output path does not have to be empty when starting.
// NOTE: Does not (yet) support symlinks or other non-file and non-dir node types.
func Sync(
	ctx context.Context,
	outPath string,
	fsHandle *unixfs.FSHandle,
	deleteMode DeleteMode,
	skipPathPrefixes []string,
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
	return SyncToBilly(ctx, outFS, fsHandle, deleteMode, skipPathPrefixes)
}
