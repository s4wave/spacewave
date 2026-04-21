package unixfs_sync

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
)

// SyncToUnixfs recursively synchronizes the contents of the UnixFS to another UnixFS.
//
// Attempts to skip files by checking size and modification time.
// The output path does not have to be empty when starting.
// Directories, regular files, and symlinks are supported; other node types
// (FIFO, device, socket, etc.) are still skipped.
func SyncToUnixfs(
	ctx context.Context,
	dest,
	src *unixfs.FSHandle,
	deleteMode DeleteMode,
	filterCb FilterCb,
) error {
	bfs := unixfs_billy.NewBillyFS(ctx, dest, "", time.Time{})
	return SyncToBilly(ctx, bfs, src, deleteMode, filterCb)
}
