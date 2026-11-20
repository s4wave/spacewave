//go:build !js && !linux

package checkoutroot

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/fsnotify"
)

// addDirectoryWatch registers the host path of the pinned directory.
// Path-based native backends need the directory name to address its entries.
func addDirectoryWatch(w *fsnotify.Watcher, root *os.Root, path string, dir *os.File) (string, error) {
	// Resolve aliases before registering so lookup and event paths agree.
	pinned, err := checkDirectory(dir)
	if err != nil {
		return "", err
	}
	name, err := filepath.EvalSymlinks(filepath.Join(root.Name(), filepath.FromSlash(path)))
	if err != nil {
		return "", err
	}
	if err := checkDirectoryIdentity(name, pinned); err != nil {
		return "", err
	}

	// Discard the registration if the path moved while the native watch opened.
	if err := w.Add(name); err != nil {
		_ = w.Remove(name) // A path-based backend may fail after installing the watch.
		return "", err
	}
	if err := checkDirectoryIdentity(name, pinned); err != nil {
		_ = w.Remove(name)
		return "", err
	}
	return name, nil
}

// checkDirectoryIdentity verifies that name still resolves to the pinned directory.
func checkDirectoryIdentity(name string, pinned fs.FileInfo) error {
	info, err := os.Stat(name)
	if err != nil {
		return err
	}
	if !os.SameFile(pinned, info) {
		return &os.PathError{Op: "watch", Path: name, Err: fs.ErrNotExist}
	}
	return nil
}
