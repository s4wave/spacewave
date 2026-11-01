//go:build darwin || linux || windows

package s4db

import (
	"context"
	"os"
)

// osFile is a database file on the operating system's file system, which
// processes share through byte locks.
type osFile struct {
	*os.File
	// path is the file's path.
	path string
	// closeFile closes the descriptor and releases its process claim.
	closeFile func() error
}

// Open opens or creates the database at path.
func Open(path string, opts Options) (*DB, error) {
	// Open the file once per process.
	f, closeFile, err := OpenFile(path, func(name string) (*os.File, error) {
		return os.OpenFile(name, os.O_RDWR|os.O_CREATE, 0o600) // #nosec G703 -- the caller chooses the path.
	})
	if err != nil {
		return nil, err
	}
	return open(&osFile{File: f, path: path, closeFile: closeFile}, opts)
}

// Build creates the database at path and writes the pairs source yields, in
// ascending key order, straight into its final shape: values over inlineMax
// into value chunks in stream order, entries into runs of runTarget at the
// lowest level whose capacity holds them, then one manifest, one superblock
// and a durable flush. No log, memtable or merge is involved, so the file
// opens with an empty memtable and nothing to merge. A key not above the one
// before it fails with ErrOutOfOrder. Build refuses an existing path and
// removes the file it created when it fails, so a partial build never passes
// for a database.
func Build(ctx context.Context, path string, opts Options, source func(yield func(key, value []byte) error) error) error {
	// Create the file and register it as open in this process.
	f, closeFile, err := OpenFile(path, func(name string) (*os.File, error) {
		return os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G703 -- the caller chooses the path.
	})
	if err != nil {
		return err
	}

	// Build into it, keeping the file only on success.
	s := &osFile{File: f, path: path, closeFile: closeFile}
	err = build(ctx, s, opts, runTarget, source)
	if cerr := s.close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path)
	}
	return err
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
	return f.closeFile()
}
