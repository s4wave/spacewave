//go:build !js

package checkoutroot

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
)

// publishFile installs a complete upload within the pinned checkout root.
func publishFile(ctx context.Context, root *os.Root, directory string, start *unixfs_rpc.OpsMknodWithContentStart, src io.Reader) (rerr error) {
	// Pin the target directory through staging, publication, and cleanup.
	dir, err := root.OpenRoot(filepath.FromSlash(directory))
	if err != nil {
		return err
	}
	defer dir.Close()

	// Stage under a private name so cancellation cannot change the destination.
	temp := ".spacewave-upload-" + rand.Text()
	file, err := dir.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		// Close first so the directory can remove the staging entry on Windows.
		_ = file.Close()
		if err := dir.Remove(temp); err != nil && !errors.Is(err, fs.ErrNotExist) {
			rerr = errors.Join(rerr, err)
		}
	}()

	// Read to the end of the RPC so a short or oversized upload never publishes.
	n, err := io.Copy(file, src)
	if err != nil {
		return err
	}
	if n != start.GetTotalSize() {
		return io.ErrUnexpectedEOF
	}

	// Apply the new mode or retain the permissions of a file being replaced.
	mode := fs.FileMode(start.GetPermissions()).Perm()
	if mode == 0 {
		mode = 0o644
	}
	if info, err := dir.Lstat(start.GetName()); err == nil && info.Mode().IsRegular() && !start.GetExclusive() {
		mode = info.Mode().Perm()
	}
	if err := file.Chmod(mode); err != nil {
		return err
	}
	ts := start.GetTimestamp().AsTime()
	if err := dir.Chtimes(temp, ts, ts); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	// Publish atomically. A hard link enforces an absent destination at the
	// same instant it makes the full file visible.
	if err := ctx.Err(); err != nil {
		return err
	}
	if start.GetExclusive() {
		if err := dir.Link(temp, start.GetName()); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return unixfs_errors.ErrExist
			}
			return err
		}
		return nil
	}
	return dir.Rename(temp, start.GetName())
}
