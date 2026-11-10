//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// dirTempPrefix starts the name of an object being written.
const dirTempPrefix = ".tmp-"

// dirObjects is a block_store_s3.ObjectStore on a local directory: a bucket
// is a directory and a key a slash path below it. It lets a PackStore write
// and read a pack set on local disk in the layout it keeps in a bucket. It
// never deletes: DeleteObject fails.
type dirObjects struct{}

// objectPath returns the file of a key.
func objectPath(bucket, key string) string {
	return filepath.Join(bucket, filepath.FromSlash(key))
}

// PutObject writes the object under a temporary name, syncs it and renames it
// into place, then syncs the directory.
func (dirObjects) PutObject(_ context.Context, bucket, key string, data []byte, _ string) error {
	// Make the object's directory.
	name := objectPath(bucket, key)
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Write and sync a temporary file, then rename it into place.
	tmp, err := os.CreateTemp(dir, dirTempPrefix)
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), name)
	}
	if err != nil {
		return errors.Wrap(err, "write "+name)
	}

	// Sync the directory so the rename is durable.
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if cerr := d.Close(); err == nil {
		err = cerr
	}
	return err
}

// GetObject reads the whole object.
func (dirObjects) GetObject(_ context.Context, bucket, key string) (io.ReadCloser, error) {
	data, err := os.ReadFile(objectPath(bucket, key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errors.Wrap(block_store_s3.ErrNotFound, key)
	}
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// GetObjectRange reads length bytes at off, fewer where the object ends.
func (dirObjects) GetObjectRange(_ context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	// Open the object's file.
	f, err := os.Open(objectPath(bucket, key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errors.Wrap(block_store_s3.ErrNotFound, key)
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read the range, short where the file ends.
	buf := make([]byte, length)
	n, err := f.ReadAt(buf, off)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf[:n], nil
}

// ListObjects calls fn with every object whose key starts with prefix, in
// name order, skipping unfinished writes.
func (dirObjects) ListObjects(_ context.Context, bucket, prefix string, fn func(key string, size int64) error) error {
	err := filepath.WalkDir(bucket, func(p string, d fs.DirEntry, err error) error {
		// Skip a missing bucket, directories and unfinished writes.
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), dirTempPrefix) {
			return nil
		}

		// Report the files under the prefix by key.
		rel, err := filepath.Rel(bucket, p)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !strings.HasPrefix(key, prefix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(key, info.Size())
	})
	return err
}

// DeleteObject refuses: the rescue never deletes.
func (dirObjects) DeleteObject(_ context.Context, _, key string) error {
	return errors.Errorf("refusing to delete %s: the rescue output is append only", key)
}

// freeBytes returns the bytes available to the user on the file system of
// path.
func freeBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil //nolint:gosec,unconvert // field types differ by OS.
}

// _ is a type assertion
var _ block_store_s3.ObjectStore = dirObjects{}
