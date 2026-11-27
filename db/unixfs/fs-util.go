package unixfs

import (
	"context"
	"io"
	"io/fs"
	"path"
	"slices"
	"time"

	"github.com/pkg/errors"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"golang.org/x/sync/errgroup"
)

// MaxReadFileSize is the maximum size for ReadFile operations, 4 GiB.
const MaxReadFileSize = 4 * 1024 * 1024 * 1024

// NewReadFileSizeTooLargeError returns a standardized error for when a file is too large to be read.
func NewReadFileSizeTooLargeError(size uint64) error {
	return errors.Errorf("file size too large for ReadFile: %d bytes", size)
}

// readdirFanOut bounds the concurrent child lookups of one directory listing.
const readdirFanOut = 8

// ReaddirAllToFileInfo calls readdir and generates FileInfo objects.
// If skip is set, skips N entries.
// If limit is set, limits output to N entries.
func ReaddirAllToFileInfo(ctx context.Context, skip, limit uint64, h *FSHandle) ([]fs.FileInfo, error) {
	// Read the entries, then drop the children removed since the listing.
	ents, infos, err := readdirInfos(ctx, skip, limit, h)
	if err != nil {
		return nil, err
	}
	out := make([]fs.FileInfo, 0, len(ents))
	for _, fi := range infos {
		if fi != nil {
			out = append(out, fi)
		}
	}
	return out, nil
}

// ReaddirAllToDirEntries calls readdir and generates fs.DirEntry objects.
// If skip is set, skips N entries.
// If limit is set, limits output to N entries.
func ReaddirAllToDirEntries(ctx context.Context, skip, limit uint64, h *FSHandle) ([]fs.DirEntry, error) {
	// Read the entries, then drop the children removed since the listing.
	ents, infos, err := readdirInfos(ctx, skip, limit, h)
	if err != nil {
		return nil, err
	}
	out := make([]fs.DirEntry, 0, len(ents))
	for i, fi := range infos {
		if fi != nil {
			out = append(out, NewFSDirEntry(ents[i], fi))
		}
	}
	return out, nil
}

// readdirInfos lists the directory entries of h and looks up the FileInfo of
// each child, up to readdirFanOut at a time. infos[i] belongs to ents[i] and is
// nil for a child removed since the listing.
func readdirInfos(ctx context.Context, skip, limit uint64, h *FSHandle) ([]FSCursorDirent, []fs.FileInfo, error) {
	// Collect directory records within the requested entry limit.
	var ents []FSCursorDirent
	err := h.ReaddirAll(ctx, skip, func(ent FSCursorDirent) error {
		ents = append(ents, ent)
		if limit != 0 && len(ents) >= int(limit) { //nolint:gosec
			return io.EOF
		}
		return nil
	})
	if err != nil && err != io.EOF {
		return nil, nil, err
	}

	// Look up the children concurrently, so their node reads overlap.
	infos := make([]fs.FileInfo, len(ents))
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(readdirFanOut)
	for i, ent := range ents {
		eg.Go(func() error {
			// Open the child, skipping one removed since the listing.
			name := ent.GetName()
			ch, err := h.Lookup(egCtx, name)
			if err == unixfs_errors.ErrNotExist {
				return nil
			}
			if err != nil {
				return errors.Wrapf(err, "lookup child %q", name)
			}
			defer ch.Release()

			// Read its info into the entry's slot.
			fi, err := ch.GetFileInfo(egCtx)
			if err != nil {
				return errors.Wrapf(err, "lookup child %q: file info", name)
			}
			infos[i] = fi
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, nil, err
	}
	return ents, infos, nil
}

// RenameWithPaths renames using two paths within a FSHandle.
func RenameWithPaths(ctx context.Context, h *FSHandle, oldPath, newPath string, ts time.Time) error {
	// Validate the source path for a relative rename.
	oldPathPts, oldPathAbsolute := SplitPath(oldPath)
	if oldPathAbsolute {
		return unixfs_errors.ErrAbsolutePath
	}

	// Validate the destination path for a relative rename.
	newPathPts, newPathAbsolute := SplitPath(newPath)
	if newPathAbsolute {
		return unixfs_errors.ErrAbsolutePath
	}

	// Leave the inode in place when both paths resolve identically.
	if slices.Equal(oldPathPts, newPathPts) {
		return nil
	}

	// Require both paths before resolving the rename handles.
	if len(newPathPts) == 0 || len(oldPathPts) == 0 {
		return unixfs_errors.ErrEmptyPath
	}

	// Open the source handle for the rename.
	oldHandle, _, err := h.LookupPathPts(ctx, oldPathPts)
	if err != nil {
		if oldHandle != nil {
			oldHandle.Release()
		}
		return err
	}
	defer oldHandle.Release()

	// Open the destination parent and retain it through the rename.
	parentPathPts := newPathPts[:len(newPathPts)-1]
	destName := newPathPts[len(newPathPts)-1]
	nextParent, _, err := h.LookupPathPts(ctx, parentPathPts)
	if err != nil {
		if nextParent != nil {
			nextParent.Release()
		}
		return err
	}
	defer nextParent.Release()

	return oldHandle.Rename(ctx, nextParent, destName, ts)
}

// StatWithPath calls Stat on a path in a FSHandle.
// StatWithPath traverses Symbolic links.
func StatWithPath(ctx context.Context, h *FSHandle, name string) (fs.FileInfo, error) {
	// Resolve a root path directly to its file information.
	name = path.Clean(name)
	if name == "." || name == "/" || name == "" {
		return h.GetFileInfo(ctx)
	}

	// Open the requested path and retain its handle through the stat.
	fh, _, err := h.LookupPath(ctx, name)
	if err != nil {
		if fh != nil {
			fh.Release()
		}
		return nil, err
	}
	defer fh.Release()

	return fh.GetFileInfo(ctx)
}

// LstatWithPath calls Lstat on a path in a FSHandle.
// Unlike StatWithPath, the final path component is not followed if it is a symlink.
// Intermediate path components are still resolved (symlinks in parent dirs are followed).
func LstatWithPath(ctx context.Context, h *FSHandle, name string) (fs.FileInfo, error) {
	// Resolve a root path directly to its file information.
	name = path.Clean(name)
	if name == "." || name == "/" || name == "" {
		return h.GetFileInfo(ctx)
	}

	// Separate the final entry from the parent path for the lstat.
	dir, base := path.Split(name)

	// Look up the parent directory (follows symlinks for intermediate components).
	var parentHandle *FSHandle
	if dir == "" || dir == "." || dir == "/" {
		var err error
		parentHandle, err = h.Clone(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		parentHandle, _, err = h.LookupPath(ctx, dir)
		if err != nil {
			if parentHandle != nil {
				parentHandle.Release()
			}
			return nil, err
		}
	}
	defer parentHandle.Release()

	// Single Lookup on the final component does NOT follow symlinks.
	childHandle, err := parentHandle.Lookup(ctx, base)
	if err != nil {
		return nil, err
	}
	defer childHandle.Release()

	return childHandle.GetFileInfo(ctx)
}

// RemoveAllWithPath calls Remove on the given path.
// Returns ErrNotExist if the path didn't exist.
func RemoveAllWithPath(ctx context.Context, h *FSHandle, filepath string, ts time.Time) error {
	// Open the containing directory for the requested removal.
	filepath = path.Clean(filepath)
	filedir, filename := path.Split(filepath)
	dirHandle, _, err := h.LookupPath(ctx, filedir)
	if err != nil {
		if dirHandle != nil {
			dirHandle.Release()
		}
		return err
	}
	defer dirHandle.Release()

	return dirHandle.Remove(ctx, []string{filename}, ts)
}

// ChmodWithPath calls Chmod on the given path.
func ChmodWithPath(ctx context.Context, h *FSHandle, filepath string, mode fs.FileMode, ts time.Time) error {
	// Open the file handle whose permissions will change.
	ch, _, err := h.LookupPath(ctx, filepath)
	if err != nil {
		if ch != nil {
			ch.Release()
		}
		return err
	}
	defer ch.Release()

	// Read the file mode before comparing its type and permissions.
	info, err := ch.GetFileInfo(ctx)
	if err != nil {
		return err
	}

	// Reject a node type change through chmod.
	oldType := info.Mode() & fs.ModeType
	setType := mode & fs.ModeType
	if oldType != setType {
		return errors.New("TODO chmod: change node type")
	}

	// Apply the requested permission bits when they differ.
	oldPerms := info.Mode() & fs.ModePerm
	setPerms := mode & fs.ModePerm
	if oldPerms != setPerms {
		err = ch.SetPermissions(ctx, setPerms, ts)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetModTimestampWithPath changes the modification timestamp on the given path.
// mtime is the modification time to use.
func SetModTimestampWithPath(ctx context.Context, h *FSHandle, filepath string, mtime time.Time) error {
	ch, _, err := h.LookupPath(ctx, filepath)
	if err != nil {
		if ch != nil {
			ch.Release()
		}
		return err
	}
	defer ch.Release()

	return ch.SetModTimestamp(ctx, mtime)
}

// ReadFile reads the named file and returns the contents.
// A successful call returns err == nil, not err == EOF.
// Because ReadFile reads the whole file, it does not treat an EOF from Read
// as an error to be reported.
func ReadFile(ctx context.Context, h *FSHandle) ([]byte, error) {
	// Read the file size to size the initial buffer.
	var size int64
	if info, err := h.GetFileInfo(ctx); err == nil {
		size = info.Size()
	}
	if size == 0 {
		return nil, nil
	}

	// If a file claims a small size, read at least 512 bytes.
	if size < 512 {
		size = 512
	} else if size > MaxReadFileSize {
		return nil, NewReadFileSizeTooLargeError(uint64(size))
	} else {
		size++ // one byte for final read at EOF
	}

	// Read the file through EOF, expanding the buffer as needed.
	data := make([]byte, 0, size)
	for {
		if len(data) >= cap(data) {
			d := append(data[:cap(data)], 0)
			data = d[:len(data)]
		}
		n, err := h.ReadAt(ctx, int64(len(data)), data[len(data):cap(data)])
		data = data[:len(data)+int(n)]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return data, err
		}
	}
}

// WriteFile writes data to the filesystem handle, which must be a file, and
// commits the file.
//
// Since WriteFile requires multiple system calls to complete, a failure
// mid-operation can leave the file in a partially written state.
func WriteFile(ctx context.Context, fsh *FSHandle, data []byte, ts time.Time) error {
	// Size chunks to the file's optimal write size.
	optimalWriteSize, err := fsh.GetOptimalWriteSize(ctx)
	if err != nil {
		return err
	}
	if optimalWriteSize == 0 {
		// copy the const here to avoid the blob import
		optimalWriteSize = 2048 * 125 // 256KB (blob.DefChunkingMinSize)
	}

	// Truncate the file.
	err = fsh.Truncate(ctx, 0, ts)
	if err != nil {
		return err
	}

	// Write data in chunks.
	for offset := int64(0); offset < int64(len(data)); offset += optimalWriteSize {
		end := min(offset+optimalWriteSize, int64(len(data)))
		chunk := data[offset:end]

		err = fsh.WriteAt(ctx, offset, chunk, ts)
		if err != nil {
			return err
		}
	}

	// Commit the buffered tail.
	return fsh.Sync(ctx)
}
