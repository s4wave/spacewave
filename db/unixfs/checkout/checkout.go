package unixfs_checkout

import (
	"context"
	"os"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
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
	// Reject use of a released filesystem handle.
	if fsHandle.CheckReleased() {
		return unixfs_errors.ErrReleased
	}

	// Reset and recreate the checkout destination.
	if _, err := os.Stat(outPath); err == nil {
		if err := os.RemoveAll(outPath); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(outPath, 0o755); err != nil {
		return err
	}

	// Delegate the UnixFS copy to the BillyFS checkout path.
	outFS := osfs.New(outPath)
	return CheckoutToBilly(ctx, outFS, fsHandle, filterCb)
}
