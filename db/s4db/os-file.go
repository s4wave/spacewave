//go:build darwin || linux || windows

package s4db

import (
	"os"

	"github.com/s4wave/spacewave/db/s4db/filelock"
)

// osFile is a database file on the operating system's file system, which
// processes share through byte locks.
type osFile struct {
	*os.File
	// path is the file's path.
	path string
	// id registers the file in the process's open files.
	id fileID
}

// Open opens or creates the database at path.
func Open(path string, opts Options) (*DB, error) {
	// Open the file once per process.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) // #nosec G703 -- the caller chooses the path.
	if err != nil {
		return nil, err
	}
	id, err := openFiles.claim(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return open(&osFile{File: f, path: path, id: id}, opts)
}

// size returns the file length.
func (f *osFile) size() (int64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// truncate sets the file length.
func (f *osFile) truncate(n int64) error {
	return f.Truncate(n)
}

// lock takes a write lock on the byte at off, waiting for other holders when
// wait is set. Without wait it reports false when another holder has it.
func (f *osFile) lock(off int64, wait bool) (bool, error) {
	return filelock.Lock(f.File, off, wait)
}

// unlock releases the lock on the byte at off.
func (f *osFile) unlock(off int64) error {
	return filelock.Unlock(f.File, off)
}

// held reports whether another holder has the lock on the byte at off.
func (f *osFile) held(off int64) (bool, error) {
	return filelock.Held(f.File, off)
}

// watch returns a watcher of the file for the reader in slot.
func (f *osFile) watch(slot int) (watcher, error) {
	w, err := newFileWatcher(f.File, f.path, slot)
	if err != nil {
		return nil, err
	}
	return w, nil
}

// close closes the file and releases its registration.
func (f *osFile) close() error {
	err := f.Close()
	openFiles.release(f.id)
	return err
}
