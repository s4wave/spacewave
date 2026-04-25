package space_http_export

import (
	"archive/zip"
	"context"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
)

// readChunkSize is the size of each read chunk when streaming file content.
const readChunkSize = 256 * 1024

// exportZip writes a zip archive of the FSHandle tree to w.
func exportZip(ctx context.Context, w io.Writer, h *unixfs.FSHandle) error {
	zw := zip.NewWriter(w)
	if err := walkAndZip(ctx, zw, h, ""); err != nil {
		zw.Close()
		return err
	}
	return zw.Close()
}

// exportNamedZip writes one selected handle into a zip archive under the given root name.
func exportNamedZip(ctx context.Context, w io.Writer, h *unixfs.FSHandle, rootName string) error {
	zw := zip.NewWriter(w)
	if err := writeHandleToZip(ctx, zw, h, rootName); err != nil {
		zw.Close()
		return err
	}
	return zw.Close()
}

// walkAndZip recursively walks the FSHandle tree and writes zip entries.
func walkAndZip(ctx context.Context, zw *zip.Writer, h *unixfs.FSHandle, prefix string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var entries []entryInfo
	err := h.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		entries = append(entries, entryInfo{
			name:      ent.GetName(),
			isDir:     ent.GetIsDirectory(),
			isSymlink: ent.GetIsSymlink(),
		})
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "readdir")
	}

	for _, entry := range entries {
		entryPath := path.Join(prefix, entry.name)
		if entry.isDir {
			if err := zipDirectory(ctx, zw, h, entry.name, entryPath); err != nil {
				return err
			}
			continue
		}

		if entry.isSymlink {
			if err := zipSymlink(ctx, zw, h, entry.name, entryPath); err != nil {
				return err
			}
			continue
		}

		if err := zipFile(ctx, zw, h, entry.name, entryPath); err != nil {
			return err
		}
	}

	return nil
}

// entryInfo holds minimal directory entry data for zip writing.
type entryInfo struct {
	name      string
	isDir     bool
	isSymlink bool
}

// zipDirectory writes a directory entry and recurses into it.
func zipDirectory(ctx context.Context, zw *zip.Writer, parent *unixfs.FSHandle, name string, entryPath string) error {
	child, err := parent.Lookup(ctx, name)
	if err != nil {
		return errors.Wrap(err, "lookup "+name)
	}
	defer child.Release()

	// Write directory entry (trailing slash).
	header := &zip.FileHeader{
		Name:     entryPath + "/",
		Method:   zip.Store,
		Modified: time.Time{},
	}
	header.SetMode(fs.ModeDir | 0o755)
	if _, err := zw.CreateHeader(header); err != nil {
		return errors.Wrap(err, "create dir header "+entryPath)
	}

	return walkAndZip(ctx, zw, child, entryPath)
}

// zipSymlink writes a symlink entry with the target as content.
func zipSymlink(ctx context.Context, zw *zip.Writer, parent *unixfs.FSHandle, name string, entryPath string) error {
	target, isAbsolute, err := parent.Readlink(ctx, name)
	if err != nil {
		return errors.Wrap(err, "readlink "+name)
	}

	targetStr := strings.Join(target, "/")
	if isAbsolute {
		targetStr = "/" + targetStr
	}

	header := &zip.FileHeader{
		Name:     entryPath,
		Method:   zip.Store,
		Modified: time.Time{},
	}
	header.SetMode(fs.ModeSymlink | 0o777)
	w, err := zw.CreateHeader(header)
	if err != nil {
		return errors.Wrap(err, "create symlink header "+entryPath)
	}
	_, err = io.WriteString(w, targetStr)
	return err
}

// zipFile writes a regular file entry with deflate compression.
func zipFile(ctx context.Context, zw *zip.Writer, parent *unixfs.FSHandle, name string, entryPath string) error {
	child, err := parent.Lookup(ctx, name)
	if err != nil {
		return errors.Wrap(err, "lookup "+name)
	}
	defer child.Release()

	info, err := child.GetFileInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "getfileinfo "+entryPath)
	}

	header := &zip.FileHeader{
		Name:               entryPath,
		Method:             zip.Deflate,
		Modified:           info.ModTime(),
		UncompressedSize64: uint64(info.Size()),
	}
	header.SetMode(info.Mode())

	w, err := zw.CreateHeader(header)
	if err != nil {
		return errors.Wrap(err, "create file header "+entryPath)
	}

	// Read file content in chunks.
	buf := make([]byte, readChunkSize)
	var offset int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		n, readErr := child.ReadAt(ctx, offset, buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return errors.Wrap(writeErr, "write "+entryPath)
			}
			offset += n
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return errors.Wrap(readErr, "read "+entryPath)
		}
		if n == 0 {
			break
		}
	}

	return nil
}
