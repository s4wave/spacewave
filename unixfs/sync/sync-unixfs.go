package unixfs_sync

import (
	"context"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
)

// SyncToUnixfs recursively synchronizes the contents of the UnixFS to another UnixFS.
//
// Attempts to skip files by checking size and modification time.
// The output path does not have to be empty when starting.
// TODO: Does not (yet) support symlinks or other non-file and non-dir node types.
func SyncToUnixfs(
	ctx context.Context,
	dest,
	src *unixfs.FSHandle,
	deleteMode DeleteMode,
	filterCb FilterCb,
) error {
	bfs := unixfs.NewBillyFS(ctx, dest, "", time.Time{})
	return SyncToBilly(ctx, bfs, src, deleteMode, filterCb)
}
