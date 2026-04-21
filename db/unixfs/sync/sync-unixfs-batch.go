package unixfs_sync

import (
	"context"
	"io"
	"path"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/pkg/errors"
)

// SyncToUnixfsBatch walks a source FSHandle and drives a BatchFSWriter,
// accumulating file content blobs eagerly and committing every directory
// merge in a single world-object transaction. Mirrors SyncToUnixfs in
// semantics but collapses the per-op 2N+1 btx.Write amplification to N+1.
//
// The destination root directory must already exist in the batch writer's
// world object (e.g. via FsInit). On error, the batch writer is Released to
// drop accumulated state; on success, Commit is called exactly once.
//
// The source handle must point at a directory; lone file or symlink sources
// are not supported.
func SyncToUnixfsBatch(
	ctx context.Context,
	b *unixfs_world.BatchFSWriter,
	src *unixfs.FSHandle,
	filterCb FilterCb,
) error {
	if src.CheckReleased() {
		b.Release()
		return unixfs_errors.ErrReleased
	}
	srcNt, err := src.GetNodeType(ctx)
	if err != nil {
		b.Release()
		return err
	}
	if !srcNt.GetIsDirectory() {
		b.Release()
		return errors.New("SyncToUnixfsBatch source must be a directory")
	}

	if err := walkBatchDir(ctx, b, src, nil, filterCb); err != nil {
		b.Release()
		return err
	}
	return b.Commit(ctx)
}

// walkBatchDir iterates children of the directory handle and forwards each
// regular file into b.AddFile. Directories are traversed recursively with an
// extended parentPath; other node types are skipped.
//
// parentPath carries the dst-relative path components for the directory
// currently being visited. The top-level call passes nil (root).
func walkBatchDir(
	ctx context.Context,
	b *unixfs_world.BatchFSWriter,
	dir *unixfs.FSHandle,
	parentPath []string,
	filterCb FilterCb,
) error {
	return dir.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		name := ent.GetName()
		if name == "" {
			return nil
		}
		if filterCb != nil {
			parts := make([]string, 0, len(parentPath)+1)
			parts = append(parts, parentPath...)
			parts = append(parts, name)
			cntu, err := filterCb(ctx, path.Join(parts...), ent)
			if err != nil || !cntu {
				return err
			}
		}

		child, err := dir.Lookup(ctx, name)
		if err != nil {
			if err == unixfs_errors.ErrNotExist {
				return nil
			}
			return err
		}
		defer child.Release()

		childNt, err := child.GetNodeType(ctx)
		if err != nil {
			return err
		}
		switch {
		case childNt.GetIsDirectory():
			perms, err := child.GetPermissions(ctx)
			if err != nil {
				return err
			}
			mtime, err := child.GetModTimestamp(ctx)
			if err != nil {
				return err
			}
			if err := b.AddDir(ctx, parentPath, name, perms, mtime); err != nil {
				return err
			}
			childPath := make([]string, 0, len(parentPath)+1)
			childPath = append(childPath, parentPath...)
			childPath = append(childPath, name)
			return walkBatchDir(ctx, b, child, childPath, filterCb)
		case childNt.GetIsFile():
			size, err := child.GetSize(ctx)
			if err != nil {
				return err
			}
			perms, err := child.GetPermissions(ctx)
			if err != nil {
				return err
			}
			mtime, err := child.GetModTimestamp(ctx)
			if err != nil {
				return err
			}
			rdr := &fsHandleReader{ctx: ctx, h: child}
			return b.AddFile(ctx, parentPath, name, childNt, int64(size), rdr, perms, mtime) //nolint:gosec
		case childNt.GetIsSymlink():
			linkPath, linkPathIsAbs, err := child.Readlink(ctx, "")
			if err != nil {
				return err
			}
			mtime, err := child.GetModTimestamp(ctx)
			if err != nil {
				return err
			}
			return b.AddSymlink(ctx, parentPath, name, linkPath, linkPathIsAbs, mtime)
		default:
			// Non-file, non-dir, non-symlink node types (FIFO, device,
			// socket) are silently skipped, matching SyncToBilly.
			return nil
		}
	})
}

// fsHandleReader adapts FSHandle.ReadAt into a sequential io.Reader so the
// blob builder can stream file contents without random access. Not safe for
// concurrent use.
type fsHandleReader struct {
	// ctx is the context used for every underlying ReadAt call.
	ctx context.Context
	// h is the source file handle.
	h *unixfs.FSHandle
	// offset tracks the next byte to read.
	offset int64
}

// Read implements io.Reader.
func (r *fsHandleReader) Read(p []byte) (int, error) {
	n, err := r.h.ReadAt(r.ctx, r.offset, p)
	r.offset += n
	return int(n), err
}

// _ is a type assertion
var _ io.Reader = ((*fsHandleReader)(nil))
