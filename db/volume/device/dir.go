//go:build !js

package device

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
)

// Dir is a Device over an operating system directory. It keeps one open
// handle per file and flushes with fsync of each file written since the last
// flush and of the directory when files were created or removed.
type Dir struct {
	// root is the directory holding the files.
	root string
	// mtx guards the fields below and serializes calls.
	mtx sync.Mutex
	// files holds the open handles by name.
	files map[string]*os.File
	// dirty holds files written or truncated since the last flush.
	dirty map[string]*os.File
	// dirDirty records a created or removed file since the last flush.
	dirDirty bool
}

// OpenDir opens a directory device, creating root when absent.
func OpenDir(root string) (*Dir, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &Dir{
		root:  root,
		files: make(map[string]*os.File),
		dirty: make(map[string]*os.File),
	}, nil
}

// Close closes every open handle without flushing.
func (d *Dir) Close() error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	var firstErr error
	for name, f := range d.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(d.files, name)
	}
	clear(d.dirty)
	return firstErr
}

// Write applies writes in order and flushes when flush is set.
func (d *Dir) Write(ctx context.Context, writes []Write, flush bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mtx.Lock()
	defer d.mtx.Unlock()
	for _, w := range writes {
		f, err := d.open(w.Name, true)
		if err != nil {
			return err
		}
		if _, err := f.WriteAt(w.Data, w.Offset); err != nil {
			return err
		}
		d.dirty[w.Name] = f
	}
	if !flush {
		return nil
	}

	// Flush file contents before the directory entries that name them.
	for name, f := range d.dirty {
		if err := f.Sync(); err != nil {
			return errors.Wrapf(err, "flush %s", name)
		}
		delete(d.dirty, name)
	}
	if !d.dirDirty {
		return nil
	}
	if err := syncDir(d.root); err != nil {
		return err
	}
	d.dirDirty = false
	return nil
}

// Read fills every read.
func (d *Dir) Read(ctx context.Context, reads []Read) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mtx.Lock()
	defer d.mtx.Unlock()
	for _, r := range reads {
		f, err := d.open(r.Name, false)
		if errors.Is(err, os.ErrNotExist) {
			return errors.Wrapf(ErrShortRead, "%s at %d", r.Name, r.Offset)
		}
		if err != nil {
			return err
		}
		_, err = f.ReadAt(r.Data, r.Offset)
		if errors.Is(err, io.EOF) {
			return errors.Wrapf(ErrShortRead, "%s at %d", r.Name, r.Offset)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Truncate sets a file's length.
func (d *Dir) Truncate(ctx context.Context, name string, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mtx.Lock()
	defer d.mtx.Unlock()
	f, err := d.open(name, true)
	if err != nil {
		return err
	}
	if err := f.Truncate(size); err != nil {
		return err
	}
	d.dirty[name] = f
	return nil
}

// Remove deletes files.
func (d *Dir) Remove(ctx context.Context, names []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mtx.Lock()
	defer d.mtx.Unlock()
	for _, name := range names {
		if err := ValidName(name); err != nil {
			return err
		}
		if f := d.files[name]; f != nil {
			_ = f.Close()
			delete(d.files, name)
			delete(d.dirty, name)
		}
		err := os.Remove(filepath.Join(d.root, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		d.dirDirty = true
	}
	return nil
}

// List returns every file in the directory.
func (d *Dir) List(ctx context.Context) ([]File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.mtx.Lock()
	defer d.mtx.Unlock()
	entries, err := os.ReadDir(d.root)
	if err != nil {
		return nil, err
	}
	files := make([]File, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if info.Mode().IsRegular() {
			files = append(files, File{Name: entry.Name(), Size: info.Size()})
		}
	}
	return files, nil
}

// open returns the handle for name, opening it when needed. With create set a
// missing file is created. The caller holds mtx.
func (d *Dir) open(name string, create bool) (*os.File, error) {
	if f := d.files[name]; f != nil {
		return f, nil
	}
	if err := ValidName(name); err != nil {
		return nil, err
	}
	path := filepath.Join(d.root, name)
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) && create {
		f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		d.dirDirty = true
	}
	if err != nil {
		return nil, err
	}
	d.files[name] = f
	return f, nil
}

// syncDir flushes a directory's entries.
func syncDir(root string) error {
	dir, err := os.Open(root)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return errors.Wrap(syncErr, "flush directory")
	}
	return closeErr
}

// _ is a type assertion
var _ Device = (*Dir)(nil)
