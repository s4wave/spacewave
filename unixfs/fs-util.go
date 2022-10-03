package unixfs

import (
	"context"
	"io"
	"io/fs"
	"path"
	"time"

	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/pkg/errors"
)

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
func RenameWithPaths(ctx context.Context, h *FSHandle, oldpath, newpath string, ts time.Time) error {
	oldpath = path.Clean(oldpath)
	newpath = path.Clean(newpath)
	if oldpath == newpath {
		return nil
	}

	newPathPts := SplitPath(newpath)
	if len(newPathPts) == 0 {
		return unixfs_errors.ErrEmptyPath
	}

	oldHandle, err := h.LookupPath(ctx, oldpath)
	if err != nil {
		return err
	}
	defer oldHandle.Release()

	parentPathPts := newPathPts[:len(newPathPts)-1]
	destName := newPathPts[len(newPathPts)-1]
	nextParent, err := h.LookupPathPts(ctx, parentPathPts)
	if err != nil {
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

	fh, err := h.LookupPath(ctx, name)
	if err != nil {
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
	dirHandle, err := h.LookupPath(ctx, filedir)
	if err != nil {
		return err
	}
	defer dirHandle.Release()

	return dirHandle.Remove(ctx, []string{filename}, ts)
}

// ChmodWithPath calls Chmod on the given path.
func ChmodWithPath(ctx context.Context, h *FSHandle, filepath string, mode fs.FileMode, ts time.Time) error {
	ch, err := h.LookupPath(ctx, filepath)
	if err != nil {
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
	ch, err := h.LookupPath(ctx, filepath)
	if err != nil {
		return err
	}
	defer ch.Release()

	return ch.SetModTimestamp(ctx, mtime)
}
