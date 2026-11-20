//go:build !js

package checkoutroot

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/aperturerobotics/fsnotify"
	"github.com/sirupsen/logrus"
)

// DirectoryWatcher shares one native watch per open directory of a checkout root.
type DirectoryWatcher struct {
	// root confines directory lookup to the served filesystem.
	root *os.Root
	// native provides one subscription to the host's directory events.
	native *fsnotify.Watcher
	// mtx serializes registrations and guards the two directory indexes.
	mtx sync.Mutex
	// dirs indexes registered directories by their relative cursor path.
	dirs map[string]*watchedDirectory
	// names indexes native event paths, which can differ from cursor paths.
	names map[string]*watchedDirectory
}

// watchedDirectory holds one native registration and the cursors that use it.
type watchedDirectory struct {
	// path is the relative path of the directory in the checkout root.
	path string
	// name is the absolute path passed to the native watcher.
	name string
	// dir pins the directory for the lifetime of the native watch.
	dir *os.File
	// listeners receives changes and directory releases, indexed by subscription.
	listeners map[*func(bool)]struct{}
}

// NewDirectoryWatcher opens the native event source for one checkout root.
func NewDirectoryWatcher(root *os.Root) (*DirectoryWatcher, error) {
	// Make one event queue for all paths and all connections on this root.
	native, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &DirectoryWatcher{
		root: root, native: native,
		dirs:  make(map[string]*watchedDirectory),
		names: make(map[string]*watchedDirectory),
	}, nil
}

// Watch registers a directory until the returned function releases its reference.
// The callback receives true if the directory itself is removed or renamed.
func (w *DirectoryWatcher) Watch(path string, cb func(bool)) (func(), error) {
	// Share a registration with every other open cursor at the same path.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	d := w.dirs[path]
	if d == nil {
		var err error
		d, err = w.openLocked(path)
		if err != nil {
			return nil, err
		}
	}

	// Retain this cursor until it is closed or the directory disappears.
	sub := &cb
	d.listeners[sub] = struct{}{}
	return func() { w.release(d, sub) }, nil
}

// Run dispatches native events until the checkout root stops.
func (w *DirectoryWatcher) Run(ctx context.Context, le *logrus.Entry) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-w.native.Events:
			if !ok {
				return fsnotify.ErrClosed
			}
			w.dispatch(event)
		case err, ok := <-w.native.Errors:
			if !ok {
				return fsnotify.ErrClosed
			}
			le.WithError(err).Warn("checkout-root directory watch")
			w.invalidateListings()
		}
	}
}

// Close ends native watches and releases the directories they pinned.
func (w *DirectoryWatcher) Close() error {
	// Stop native registration before releasing the open directory descriptors.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	err := w.native.Close()
	for _, dir := range w.dirs {
		err = errors.Join(err, dir.dir.Close())
	}
	clear(w.dirs)
	clear(w.names)
	return err
}

// openLocked installs one native watch for the pinned directory at path.
func (w *DirectoryWatcher) openLocked(path string) (*watchedDirectory, error) {
	// Open the watched directory through the same confinement as file reads.
	dir, err := w.root.Open(filepath.FromSlash(path))
	if err != nil {
		return nil, err
	}
	name, err := addDirectoryWatch(w.native, w.root, path, dir)
	if err != nil {
		return nil, errors.Join(err, dir.Close())
	}

	// Index the registration by both cursor path and native event path.
	d := &watchedDirectory{path: path, name: name, dir: dir, listeners: make(map[*func(bool)]struct{})}
	w.dirs[path], w.names[name] = d, d
	return d, nil
}

// release drops the kernel watch after the last cursor releases the directory.
func (w *DirectoryWatcher) release(d *watchedDirectory, sub *func(bool)) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	delete(d.listeners, sub)
	if len(d.listeners) == 0 && w.dirs[d.path] == d {
		w.removeLocked(d)
	}
}

// removeLocked retires a directory registration and closes its pinned handle.
func (w *DirectoryWatcher) removeLocked(d *watchedDirectory) {
	// The kernel may have already removed a watch on a deleted directory.
	delete(w.dirs, d.path)
	delete(w.names, d.name)
	_ = w.native.Remove(d.name)
	_ = d.dir.Close()
}

// dispatch wakes the watched directory and the watched parent of the event path.
func (w *DirectoryWatcher) dispatch(event fsnotify.Event) {
	// Route the event to at most the watch at its path and its parent.
	w.mtx.Lock()
	self := w.names[event.Name]
	parent := w.names[filepath.Dir(event.Name)]
	removed := self != nil && event.Has(fsnotify.Remove|fsnotify.Rename) && !w.rearmLocked(self)
	callbacks := make([]*func(bool), 0, listenerCount(self)+listenerCount(parent))

	// Retire a directory that went away while retaining its listeners for dispatch.
	if self != nil {
		for cb := range self.listeners {
			callbacks = append(callbacks, cb)
		}
		if removed {
			w.removeLocked(self)
		}
	}
	selfCount := len(callbacks)

	// The parent still exists and needs only a fresh listing.
	if parent != nil && parent != self {
		for cb := range parent.listeners {
			callbacks = append(callbacks, cb)
		}
	}
	w.mtx.Unlock()

	// Notify the RPC cursors outside the registry lock.
	for i, cb := range callbacks {
		(*cb)(removed && i < selfCount)
	}
}

// invalidateListings rechecks open directories after the native source lost events.
func (w *DirectoryWatcher) invalidateListings() {
	// Re-arm watches after overflow and separate surviving from removed directories.
	w.mtx.Lock()
	var changed, removed []*func(bool)
	for _, dir := range w.dirs {
		if w.rearmLocked(dir) {
			for cb := range dir.listeners {
				changed = append(changed, cb)
			}
			continue
		}
		for cb := range dir.listeners {
			removed = append(removed, cb)
		}
		w.removeLocked(dir)
	}
	w.mtx.Unlock()

	// Notify the RPC cursors outside the registry lock.
	for _, cb := range changed {
		(*cb)(false)
	}
	for _, cb := range removed {
		(*cb)(true)
	}
}

// rearmLocked keeps a watch only while the cursor still names its directory.
func (w *DirectoryWatcher) rearmLocked(d *watchedDirectory) bool {
	// A queued remove or rename can refer to a previous watch of this path.
	pinned, err := d.dir.Stat()
	if err != nil {
		return false
	}
	current, err := w.root.Stat(filepath.FromSlash(d.path))
	if err != nil || !os.SameFile(current, pinned) {
		return false
	}

	// Re-register through the pinned directory if the backend dropped the watch.
	name, err := addDirectoryWatch(w.native, w.root, d.path, d.dir)
	if err != nil {
		return false
	}
	if name != d.name {
		delete(w.names, d.name)
		_ = w.native.Remove(d.name)
		d.name = name
		w.names[name] = d
	}
	return true
}

// listenerCount returns how many cursors will receive an event for d.
func listenerCount(d *watchedDirectory) int {
	if d == nil {
		return 0
	}
	return len(d.listeners)
}

// checkDirectory returns info about the pinned directory to be watched.
func checkDirectory(dir *os.File) (fs.FileInfo, error) {
	info, err := dir.Stat()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "watch", Path: dir.Name(), Err: fs.ErrInvalid}
	}
	return info, nil
}
