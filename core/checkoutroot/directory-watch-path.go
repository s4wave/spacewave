//go:build !js && !linux

package checkoutroot

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/fsnotify"
)

// directoryWatchKey identifies the host path used by a path-based watcher.
type directoryWatchKey = string

// directoryWatchName is the host path reported by the native source.
type directoryWatchName = string

// directoryWatchSource dispatches notifications from a path-based backend.
type directoryWatchSource struct {
	// watcher owns the native registrations and event channels.
	watcher *fsnotify.Watcher
}

// directoryKey resolves aliases to the single path the backend will register.
func directoryKey(root *os.Root, path string, pinned fs.FileInfo) (directoryWatchKey, error) {
	// Resolve the path and check that it still points to the confined directory.
	name, err := filepath.EvalSymlinks(filepath.Join(root.Name(), filepath.FromSlash(path)))
	if err != nil {
		return "", err
	}
	return name, checkDirectoryIdentity(name, pinned)
}

// openDirectoryWatchSource opens the native queue for the checkout root.
func openDirectoryWatchSource() (*directoryWatchSource, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &directoryWatchSource{watcher: watcher}, nil
}

// add watches the path only while it resolves to the pinned directory.
func (s *directoryWatchSource) add(name directoryWatchKey, _ *os.File, pinned fs.FileInfo) (directoryWatchName, error) {
	// Check the path on both sides of the native registration.
	if err := checkDirectoryIdentity(name, pinned); err != nil {
		return "", err
	}
	if err := s.watcher.Add(name); err != nil {
		s.remove(name) // kqueue can fail after opening some of the descriptors.
		return "", err
	}
	if err := checkDirectoryIdentity(name, pinned); err != nil {
		s.remove(name)
		return "", err
	}
	return name, nil
}

// remove retires the watch if the backend still holds it.
func (s *directoryWatchSource) remove(name directoryWatchName) {
	_ = s.watcher.Remove(name)
}

// close ends the source and releases its registrations.
func (s *directoryWatchSource) close() error {
	return s.watcher.Close()
}

// run dispatches structural events and reconciles overflow from the native queue.
func (s *directoryWatchSource) run(ctx context.Context, w *DirectoryWatcher) error {
	// The fork delivers its default mask on these backends. Only structural
	// events reach the registry because these backends lack close-write.
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-s.watcher.Events:
			if !ok {
				return fsnotify.ErrClosed
			}
			if !event.Has(fsnotify.Create | fsnotify.Remove | fsnotify.Rename) {
				continue
			}
			w.dispatch(event.Name, event.Has(fsnotify.Remove|fsnotify.Rename))
			if parent := filepath.Dir(event.Name); parent != event.Name {
				w.dispatch(parent, false)
			}
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return fsnotify.ErrClosed
			}
			if !errors.Is(err, fsnotify.ErrEventOverflow) {
				return err
			}
			w.overflow()
		}
	}
}

// checkDirectoryIdentity verifies that name still resolves to the pinned directory.
func checkDirectoryIdentity(name string, pinned fs.FileInfo) error {
	// Reject a native path that moved or changed between resolution and registration.
	info, err := os.Stat(name)
	if err != nil {
		return err
	}
	if !os.SameFile(pinned, info) {
		return &os.PathError{Op: "watch", Path: name, Err: fs.ErrNotExist}
	}
	return nil
}
