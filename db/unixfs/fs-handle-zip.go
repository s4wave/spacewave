package unixfs

import (
	"archive/zip"
	"context"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// zipReadChunkSize is the size of each read chunk when streaming file
// content into a zip archive.
const zipReadChunkSize = 256 * 1024

// WriteZipArchive writes a zip archive of the FSHandle tree to w. An empty
// rootName archives the handle's full contents; otherwise the handle itself
// is written under rootName.
func WriteZipArchive(ctx context.Context, w io.Writer, h *FSHandle, rootName string) error {
	zw := zip.NewWriter(w)
	if err := WriteFSHandleToZip(ctx, zw, h, rootName); err != nil {
		zw.Close()
		return err
	}
	return zw.Close()
}

// WriteFSHandleToZip writes one FSHandle into an existing zip writer under
// rootName: directories recurse into their contents, symlinks serialize
// their target, and regular files stream with deflate compression.
func WriteFSHandleToZip(ctx context.Context, zw *zip.Writer, handle *FSHandle, rootName string) error {
	info, err := handle.GetFileInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "stat "+rootName)
	}

	if info.IsDir() {
		cleanPrefix := strings.TrimSuffix(rootName, "/")
		if cleanPrefix != "" {
			header := &zip.FileHeader{
				Name:   cleanPrefix + "/",
				Method: zip.Store,
			}
			header.SetMode(fs.ModeDir | 0o755)
			if _, err := zw.CreateHeader(header); err != nil {
				return errors.Wrap(err, "create dir header "+cleanPrefix)
			}
		}
		return walkAndZip(ctx, zw, handle, cleanPrefix)
	}

	if info.Mode()&fs.ModeSymlink != 0 {
		target, isAbsolute, err := handle.Readlink(ctx, "")
		if err != nil {
			return errors.Wrap(err, "readlink "+rootName)
		}
		targetPath := strings.Join(target, "/")
		if isAbsolute {
			targetPath = "/" + targetPath
		}
		header := &zip.FileHeader{Name: rootName, Method: zip.Store}
		header.SetMode(fs.ModeSymlink | 0o777)
		w, err := zw.CreateHeader(header)
		if err != nil {
			return errors.Wrap(err, "create symlink header "+rootName)
		}
		_, err = io.WriteString(w, targetPath)
		return err
	}

	return writeFileToZip(ctx, zw, handle, path.Clean(rootName), info)
}

// walkAndZip recursively walks the FSHandle tree and writes zip entries.
func walkAndZip(ctx context.Context, zw *zip.Writer, h *FSHandle, prefix string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	type dirent struct {
		name      string
		isDir     bool
		isSymlink bool
	}

	var entries []dirent
	err := h.ReaddirAll(ctx, 0, func(ent FSCursorDirent) error {
		entries = append(entries, dirent{
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

// zipDirectory writes a directory entry and recurses into it.
func zipDirectory(ctx context.Context, zw *zip.Writer, parent *FSHandle, name string, entryPath string) error {
	child, err := parent.Lookup(ctx, name)
	if err != nil {
		return errors.Wrap(err, "lookup "+name)
	}
	defer child.Release()

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
func zipSymlink(ctx context.Context, zw *zip.Writer, parent *FSHandle, name string, entryPath string) error {
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
func zipFile(ctx context.Context, zw *zip.Writer, parent *FSHandle, name string, entryPath string) error {
	child, err := parent.Lookup(ctx, name)
	if err != nil {
		return errors.Wrap(err, "lookup "+name)
	}
	defer child.Release()

	info, err := child.GetFileInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "getfileinfo "+entryPath)
	}
	return writeFileToZip(ctx, zw, child, entryPath, info)
}

// writeFileToZip streams one file handle into a zip entry with deflate
// compression.
func writeFileToZip(ctx context.Context, zw *zip.Writer, handle *FSHandle, entryPath string, info fs.FileInfo) error {
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

	buf := make([]byte, zipReadChunkSize)
	var offset int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		n, readErr := handle.ReadAt(ctx, offset, buf)
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
