package unixfs_checkout

import (
	"context"
	"os"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	unixfs_sync "github.com/aperturerobotics/hydra/unixfs/sync"
	"github.com/go-git/go-billy/v6/osfs"
)

// Checkout recursively copies the contents of the UnixFS to disk.
//
// Assumes that the output path is empty when starting.
// NOTE: Does not (yet) support symlinks or other non-file and non-dir node types.
func Checkout(
	ctx context.Context,
	outPath string,
	fsHandle *unixfs.FSHandle,
	filterCb unixfs_sync.FilterCb,
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
	if err := os.MkdirAll(outPath, 0o755); err != nil {
		return err
	}

	// construct a BillyFS at the outPath & checkout
	outFS := osfs.New(outPath)
	return CheckoutToBilly(ctx, outFS, fsHandle, filterCb)
}
