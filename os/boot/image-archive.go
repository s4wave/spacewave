//go:build linux

package boot

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
)

// importImageArchive streams an explicit application bundle into a new image.
// It accepts regular files and directories only, keeping links and special
// devices out of the import. The selected immutable root covers their bytes.
func importImageArchive(ctx context.Context, root *unixfs.FSHandle, source io.Reader) error {
	// Consume one archive entry at a time, never collecting the application tree.
	archive := tar.NewReader(source)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(header.Name)
		if name == "." {
			continue
		}
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return errors.New("image archive entry escapes the root: " + header.Name)
		}
		if err := importImageEntry(ctx, root, name, header, archive); err != nil {
			return err
		}
	}
}

// importImageEntry writes a single regular file or creates its parent directories.
func importImageEntry(ctx context.Context, root *unixfs.FSHandle, name string, header *tar.Header, source io.Reader) error {
	// Directories may already exist as part of the fixed shell image.
	ts := time.Now()
	perm := header.FileInfo().Mode().Perm()
	if header.Typeflag == tar.TypeDir {
		return root.MkdirAllPath(ctx, name, perm, ts)
	}
	if header.Typeflag != tar.TypeReg {
		return errors.New("image archive requires regular files: " + name)
	}
	if err := root.MkdirAllPath(ctx, path.Dir(name), 0o755, ts); err != nil {
		return err
	}

	// Stream this file through the ordinary UnixFS write operation.
	parent, _, err := root.LookupPath(ctx, path.Dir(name))
	if err != nil {
		return err
	}
	defer parent.Release()
	return parent.MknodWithContent(ctx, path.Base(name), unixfs.NewFSCursorNodeType_File(), header.Size, source, perm, ts)
}
