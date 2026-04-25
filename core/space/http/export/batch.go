package space_http_export

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"encoding/base64"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
)

func decodeBatchRequest(payload string) ([]string, error) {
	encoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, errors.Wrap(err, "decode batch payload")
	}

	if data, err := inflateData(encoded); err == nil {
		paths, err := decodeBatchRequestData(data)
		if err == nil {
			return paths, nil
		}
	}
	return decodeBatchRequestData(encoded)
}

func decodeBatchRequestData(data []byte) ([]string, error) {
	var req ExportBatchRequest
	if err := req.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode batch request")
	}
	return normalizeBatchPaths(req.GetPaths())
}

func inflateData(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	if out, err := readCompressed(zlib.NewReader(bytes.NewReader(data))); err == nil {
		return out, nil
	}
	return readCompressed(flate.NewReader(bytes.NewReader(data)), nil)
}

func readCompressed(reader io.ReadCloser, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func exportBatchZip(ctx context.Context, w io.Writer, baseHandle *unixfs.FSHandle, relPaths []string) error {
	normalizedPaths, err := normalizeBatchPaths(relPaths)
	if err != nil {
		return err
	}

	zw := zip.NewWriter(w)
	for _, relPath := range normalizedPaths {
		targetHandle, _, err := baseHandle.LookupPath(ctx, relPath)
		if err != nil {
			zw.Close()
			return errors.Wrap(err, "lookup batch path "+relPath)
		}

		if err := writeHandleToZip(ctx, zw, targetHandle, relPath); err != nil {
			targetHandle.Release()
			zw.Close()
			return err
		}
		targetHandle.Release()
	}

	return zw.Close()
}

func normalizeBatchPaths(relPaths []string) ([]string, error) {
	if len(relPaths) == 0 {
		return nil, errors.New("batch request requires at least one path")
	}

	seen := make(map[string]struct{}, len(relPaths))
	normalized := make([]string, 0, len(relPaths))
	for _, relPath := range relPaths {
		cleanPath, err := normalizeBatchPath(relPath)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[cleanPath]; ok {
			continue
		}
		seen[cleanPath] = struct{}{}
		normalized = append(normalized, cleanPath)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizeBatchPath(relPath string) (string, error) {
	trimmedPath := strings.TrimSpace(relPath)
	if trimmedPath == "" {
		return "", errors.New("batch path is empty")
	}
	if strings.HasPrefix(trimmedPath, "/") {
		return "", errors.New("batch path must be relative")
	}

	cleanPath := path.Clean(trimmedPath)
	if cleanPath == "." {
		return "", errors.New("batch path must identify a descendant")
	}
	if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
		return "", errors.New("batch path escapes base path")
	}
	return cleanPath, nil
}

func writeHandleToZip(ctx context.Context, zw *zip.Writer, handle *unixfs.FSHandle, prefix string) error {
	info, err := handle.GetFileInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "stat "+prefix)
	}

	if info.IsDir() {
		cleanPrefix := strings.TrimSuffix(prefix, "/")
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
			return errors.Wrap(err, "readlink "+prefix)
		}
		targetPath := strings.Join(target, "/")
		if isAbsolute {
			targetPath = "/" + targetPath
		}
		header := &zip.FileHeader{Name: prefix, Method: zip.Store}
		header.SetMode(fs.ModeSymlink | 0o777)
		w, err := zw.CreateHeader(header)
		if err != nil {
			return errors.Wrap(err, "create symlink header "+prefix)
		}
		_, err = io.WriteString(w, targetPath)
		return err
	}

	return writeFileHandleToZip(ctx, zw, handle, prefix, info)
}

func writeFileHandleToZip(ctx context.Context, zw *zip.Writer, handle *unixfs.FSHandle, entryPath string, info fs.FileInfo) error {
	header := &zip.FileHeader{
		Name:               path.Clean(entryPath),
		Method:             zip.Deflate,
		Modified:           info.ModTime(),
		UncompressedSize64: uint64(info.Size()),
	}
	header.SetMode(info.Mode())

	w, err := zw.CreateHeader(header)
	if err != nil {
		return errors.Wrap(err, "create file header "+entryPath)
	}

	buf := make([]byte, readChunkSize)
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
