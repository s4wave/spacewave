//go:build !js

package checkoutroot

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	pkgerrors "github.com/pkg/errors"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
)

// publishFile installs a complete upload within the pinned checkout root.
func publishFile(ctx context.Context, root *os.Root, journal *uploadJournal, directory string, start *unixfs_rpc.OpsMknodWithContentStart, src io.Reader) (rerr error) {
	// Pin the target directory and use a filename valid on this host.
	name, err := filepath.Localize(start.GetName())
	if err != nil {
		return err
	}
	if hiddenUploadName(name) {
		return errors.New("upload name is reserved for staging")
	}
	path, err := filepath.Localize(directory)
	if err != nil {
		return err
	}
	dir, err := root.OpenRoot(path)
	if err != nil {
		return err
	}
	defer dir.Close()

	// Apply the new mode or retain the permissions of a file being replaced.
	mode := fs.FileMode(start.GetPermissions()).Perm()
	if mode == 0 {
		mode = 0o644
	}
	if info, err := dir.Lstat(name); err == nil && info.Mode().IsRegular() && !start.GetExclusive() {
		mode = info.Mode().Perm()
	}
	ts := start.GetTimestamp().AsTime()

	// An exclusive upload can publish directly from an anonymous inode.
	if start.GetExclusive() {
		file, err := openAnonymousStage(dir)
		if err != nil && !errors.Is(err, errors.ErrUnsupported) {
			return err
		}
		if file != nil {
			defer file.Close()
			if err := writeStage(file, src, start.GetTotalSize(), mode); err != nil {
				return err
			}
			err = publishAnonymousStage(ctx, dir, file, name, ts)
			if !errors.Is(err, errors.ErrUnsupported) {
				return publicationResult(err)
			}

			// Keep the complete stream locally if this mount cannot link it.
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return err
			}
			src = file
		}
	}

	// Give the named fallback a recoverable lease before writing upload bytes.
	stage, err := openNamedStage(ctx, journal, dir, directory)
	if err != nil {
		return err
	}
	defer func() {
		// A committed file stays successful if the next sweep must finish cleanup.
		if err := stage.discard(ctx); rerr != nil {
			rerr = errors.Join(rerr, err)
		}
	}()
	if err := writeStage(stage.file, src, start.GetTotalSize(), mode); err != nil {
		return err
	}
	if err := dir.Chtimes(stage.name, ts, ts); err != nil {
		return err
	}
	if err := stage.file.Close(); err != nil {
		return err
	}

	// Publish the complete file with the caller's replacement rule.
	if err := ctx.Err(); err != nil {
		return err
	}
	if start.GetExclusive() {
		return publicationResult(renameExclusive(dir, stage.name, name))
	}
	return dir.Rename(stage.name, name)
}

// writeStage validates the whole stream before it can be published.
func writeStage(file *os.File, src io.Reader, size int64, mode fs.FileMode) error {
	// Read through the stream end and verify the declared size.
	n, err := io.Copy(file, src)
	if err != nil {
		return err
	}
	if n != size {
		return io.ErrUnexpectedEOF
	}

	// Apply the destination mode before making the file visible.
	return file.Chmod(mode)
}

// publicationResult maps the filesystem's exclusive conflict to the RPC contract.
func publicationResult(err error) error {
	if errors.Is(err, fs.ErrExist) {
		return unixfs_errors.ErrExist
	}
	return pkgerrors.Wrap(err, "publish upload without replacing the destination")
}
