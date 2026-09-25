//go:build js

// Package device_opfs implements device.Device on an OPFS directory with sync
// access handles, which only a dedicated worker can open.
//
// Each file keeps one sync access handle open from first use until Remove or
// Close. A flush calls flush() on every handle written or truncated since the
// last flush.
package device_opfs

import (
	"context"
	"strings"
	"sync"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/device"
)

// Device is a device.Device on one OPFS directory.
type Device struct {
	// dir is the directory handle.
	dir js.Value

	// mtx serializes the device calls.
	mtx sync.Mutex
	// files holds the open handle of each file.
	files map[string]*opfs.SyncFile
	// dirty holds the files changed since the last flush.
	dirty map[string]bool
}

// Open opens the directory at the slash-separated path under the OPFS root,
// creating it when absent.
func Open(path string) (*Device, error) {
	root, err := opfs.GetRoot()
	if err != nil {
		return nil, err
	}
	dir, err := opfs.GetDirectoryPath(root, strings.Split(path, "/"), true)
	if err != nil {
		return nil, err
	}
	return &Device{dir: dir, files: make(map[string]*opfs.SyncFile), dirty: make(map[string]bool)}, nil
}

// Delete removes the directory at path and everything in it. A missing
// directory is not an error.
func Delete(path string) error {
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}
	parts := strings.Split(path, "/")
	parent, err := opfs.GetDirectoryPath(root, parts[:len(parts)-1], false)
	if opfs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	err = opfs.DeleteEntry(parent, parts[len(parts)-1], true)
	if opfs.IsNotFound(err) {
		return nil
	}
	return err
}

// Write applies writes in order and flushes every changed file if flush is
// set.
func (d *Device) Write(ctx context.Context, writes []device.Write, flush bool) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	// Write each range through its file's handle.
	for _, w := range writes {
		if err := device.ValidName(w.Name); err != nil {
			return err
		}
		f, err := d.open(w.Name, true)
		if err != nil {
			return err
		}
		if _, err := f.WriteAt(w.Data, w.Offset); err != nil {
			return err
		}
		d.dirty[w.Name] = true
	}
	if !flush {
		return nil
	}

	// Flush every file changed since the last flush.
	for name := range d.dirty {
		if f := d.files[name]; f != nil {
			f.Flush()
		}
	}
	clear(d.dirty)
	return nil
}

// Read fills every read.
func (d *Device) Read(ctx context.Context, reads []device.Read) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	for _, r := range reads {
		f, err := d.open(r.Name, false)
		if err != nil {
			return err
		}
		if f == nil || r.Offset+int64(len(r.Data)) > f.Size() {
			return errors.Wrapf(device.ErrShortRead, "%s at %d", r.Name, r.Offset)
		}
		if len(r.Data) == 0 {
			continue
		}
		n, err := f.ReadAt(r.Data, r.Offset)
		if err != nil {
			return err
		}
		if n != len(r.Data) {
			return errors.Wrapf(device.ErrShortRead, "%s at %d", r.Name, r.Offset)
		}
	}
	return nil
}

// Truncate sets a file's length, creating it when absent.
func (d *Device) Truncate(ctx context.Context, name string, size int64) error {
	if err := device.ValidName(name); err != nil {
		return err
	}
	d.mtx.Lock()
	defer d.mtx.Unlock()
	f, err := d.open(name, true)
	if err != nil {
		return err
	}
	f.Truncate(size)
	d.dirty[name] = true
	return nil
}

// Remove closes and deletes files.
func (d *Device) Remove(ctx context.Context, names []string) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	for _, name := range names {
		if err := device.ValidName(name); err != nil {
			return err
		}
		if f := d.files[name]; f != nil {
			_ = f.Close()
			delete(d.files, name)
		}
		delete(d.dirty, name)
		if err := opfs.DeleteEntry(d.dir, name, false); err != nil && !opfs.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// List returns every file, opening a handle on each to read its size.
func (d *Device) List(ctx context.Context) ([]device.File, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	names, err := opfs.ListDirectory(d.dir)
	if err != nil {
		return nil, err
	}
	out := make([]device.File, 0, len(names))
	for _, name := range names {
		f, err := d.open(name, false)
		if err != nil {
			return nil, err
		}
		if f != nil {
			out = append(out, device.File{Name: name, Size: f.Size()})
		}
	}
	return out, nil
}

// Close closes every handle.
func (d *Device) Close() error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	for name, f := range d.files {
		_ = f.Close()
		delete(d.files, name)
	}
	return nil
}

// open returns the handle of name, opening it on first use. A missing file
// returns nil unless create is set.
func (d *Device) open(name string, create bool) (*opfs.SyncFile, error) {
	if f := d.files[name]; f != nil {
		return f, nil
	}
	open := opfs.OpenSyncFile
	if create {
		open = opfs.CreateSyncFile
	}
	f, err := open(d.dir, name)
	if !create && opfs.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, name)
	}
	d.files[name] = f
	return f, nil
}

// _ checks that Device is a device.Device.
var _ device.Device = (*Device)(nil)
