package unixfs

import (
	"context"
	"io"
	"io/fs"
	"path"
	"slices"
	"time"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/pkg/errors"
)

// MaxReadFileSize is the maximum size for ReadFile operations (4GB)
const MaxReadFileSize = 4 * 1024 * 1024 * 1024

// NewReadFileSizeTooLargeError returns a standardized error for when a file is too large to be read.
func NewReadFileSizeTooLargeError(size uint64) error {
	return errors.Errorf("file size too large for ReadFile: %d bytes", size)
}

// ReaddirAllToFileInfo calls readdir and generates FileInfo objects.
// If skip is set, skips N entries.
// If limit is set, limits output to N entries.
func ReaddirAllToFileInfo(ctx context.Context, skip, limit uint64, h *FSHandle) ([]fs.FileInfo, error) {
	var children []string
	err := h.ReaddirAll(ctx, skip, func(ent FSCursorDirent) error {
		children = append(children, ent.GetName())
		if limit != 0 && len(children) >= int(limit) {
			return io.EOF
		}
		return nil
	})
	if err != nil && err != io.EOF {
		return nil, err
	}

	// lookup all children
	out := make([]fs.FileInfo, 0, len(children))
	for _, childName := range children {
		ch, err := h.Lookup(ctx, childName)
		if err == unixfs_errors.ErrNotExist {
			continue
		}
		if err != nil {
			return nil, errors.Wrapf(err, "lookup child %q", childName)
		}
		fi, err := ch.GetFileInfo(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "lookup child %q: file info", childName)
		}
		out = append(out, fi)
	}
	return out, nil
}

// ReaddirAllToFSDirEntry calls readdir and generates FSDirEntry objects.
// If skip is set, skips N entries.
// If limit is set, limits output to N entries.
func ReaddirAllToDirEntries(ctx context.Context, skip, limit uint64, h *FSHandle) ([]fs.DirEntry, error) {
	var children []FSCursorDirent
	err := h.ReaddirAll(ctx, skip, func(ent FSCursorDirent) error {
		children = append(children, ent)
		if limit != 0 && len(children) >= int(limit) {
			return io.EOF
		}
		return nil
	})
	if err != nil && err != io.EOF {
		return nil, err
	}

	// lookup all children & build directory entries
	out := make([]fs.DirEntry, 0, len(children))
	for _, child := range children {
		childName := child.GetName()
		ch, err := h.Lookup(ctx, childName)
		if err == unixfs_errors.ErrNotExist {
			continue
		}
		if err != nil {
			return nil, errors.Wrapf(err, "lookup child %q", childName)
		}
		fi, err := ch.GetFileInfo(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "lookup child %q: file info", childName)
		}
		out = append(out, NewFSDirEntry(child, fi))
	}

	return out, nil
}

// RenameWithPaths renames using two paths within a FSHandle.
func RenameWithPaths(ctx context.Context, h *FSHandle, oldPath, newPath string, ts time.Time) error {
	oldPathPts, oldPathAbsolute := SplitPath(oldPath)
	if oldPathAbsolute {
		return unixfs_errors.ErrAbsolutePath
	}

	newPathPts, newPathAbsolute := SplitPath(newPath)
	if newPathAbsolute {
		return unixfs_errors.ErrAbsolutePath
	}

	if slices.Equal(oldPathPts, newPathPts) {
		return nil
	}

	if len(newPath) == 0 || len(oldPath) == 0 {
		return unixfs_errors.ErrEmptyPath
	}

	oldHandle, _, err := h.LookupPathPts(ctx, oldPathPts)
	if err != nil {
		if oldHandle != nil {
			oldHandle.Release()
		}
		return err
	}
	defer oldHandle.Release()

	parentPathPts := newPathPts[:len(newPath)-1]
	destName := newPathPts[len(newPath)-1]
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
// Note: this will traverse Symbolic links.
// TODO: LStat (don't traverse symbolic links.)
func StatWithPath(ctx context.Context, h *FSHandle, name string) (fs.FileInfo, error) {
	name = path.Clean(name)
	if name == "." || name == "/" || name == "" {
		return h.GetFileInfo(ctx)
	}

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

// RemoveAllWithPath calls Remove on the given path.
// Returns ErrNotExist if the path didn't exist.
func RemoveAllWithPath(ctx context.Context, h *FSHandle, filepath string, ts time.Time) error {
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
	ch, _, err := h.LookupPath(ctx, filepath)
	if err != nil {
		if ch != nil {
			ch.Release()
		}
		return err
	}
	defer ch.Release()

	info, err := ch.GetFileInfo(ctx)
	if err != nil {
		return err
	}

	oldType := info.Mode() & fs.ModeType
	setType := mode & fs.ModeType
	if oldType != setType {
		return errors.New("TODO chmod: change node type")
	}

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

// WriteFile writes data to the filesystem handle, which must be a file.
//
// Since WriteFile requires multiple system calls to complete, a failure mid-operation
// can leave the file in a partially written state.
func WriteFile(ctx context.Context, fsh *FSHandle, data []byte, ts time.Time) error {
	optimalWriteSize, err := fsh.GetOptimalWriteSize(ctx)
	if err != nil {
		return err
	}
	if optimalWriteSize == 0 {
		// copy the const here to avoid the blob import
		optimalWriteSize = 2048 * 125 // 256KB - blob.DefChunkingMinSize
	}

	// Truncate the file
	err = fsh.Truncate(ctx, 0, ts)
	if err != nil {
		return err
	}

	// Write data in chunks
	for offset := int64(0); offset < int64(len(data)); offset += int64(optimalWriteSize) {
		end := min(offset+int64(optimalWriteSize), int64(len(data)))
		chunk := data[offset:end]

		err = fsh.WriteAt(ctx, offset, chunk, ts)
		if err != nil {
			return err
		}
	}

	return nil
}
