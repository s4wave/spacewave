package unixfs_sync

import (
	"io/fs"
	"os"

	"github.com/go-git/go-billy/v6"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// prepareSyncDirectory leaves existing directories unchanged and makes a new
// directory writable while its children are copied. Its returned final mode
// respects the caller's umask; nil means no final permission change is needed.
// The caller restores that mode after the children or on a failed partial copy.
func prepareSyncDirectory(bfs BillyFS, name string, sourceMode fs.FileMode) (*fs.FileMode, error) {
	// An existing directory, including the output root, keeps its permissions.
	info, err := bfs.Stat(name)
	if err == nil && info.IsDir() {
		return nil, nil
	}
	if err != nil && !os.IsNotExist(err) && err != unixfs_errors.ErrNotExist {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}

	// A filesystem without chmod retains its existing mkdir contract.
	chmod, canChmod := bfs.(billy.Chmod)
	if !canChmod || name == "" || err == nil {
		return nil, bfs.MkdirAll(name, sourceMode)
	}
	if err := bfs.MkdirAll(name, sourceMode|0o700); err != nil {
		return nil, &fs.PathError{Op: "mkdir", Path: name, Err: err}
	}

	// Remove only the temporary owner permissions, preserving the applied umask.
	created, err := bfs.Stat(name)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}
	finalMode := created.Mode().Perm() & sourceMode
	writableMode := created.Mode().Perm() | 0o700
	if writableMode == finalMode {
		return nil, nil
	}

	// An unusually restrictive umask must not prevent populating a new directory.
	if created.Mode().Perm() != writableMode {
		if err := chmod.Chmod(name, writableMode); err != nil {
			return nil, &fs.PathError{Op: "chmod", Path: name, Err: err}
		}
	}
	return &finalMode, nil
}
