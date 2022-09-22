package unixfs_checkout

import (
	"context"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_sync "github.com/aperturerobotics/hydra/unixfs/sync"
)

// BillyFS has the needed billy filesystem interfaces.
type BillyFS = unixfs_sync.BillyFS

// CheckoutToBilly recursively copies the contents of the UnixFS to a BillyFS.
//
// Assumes that the output path is empty when starting.
// NOTE: Does not (yet) support symlinks or other non-file and non-dir node types.
func CheckoutToBilly(ctx context.Context, bfs BillyFS, fsHandle *unixfs.FSHandle) error {
	return unixfs_sync.SyncToBilly(ctx, bfs, fsHandle, unixfs_sync.DeleteMode_DeleteMode_NONE)
}
