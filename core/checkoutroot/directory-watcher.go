//go:build !js

package checkoutroot

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/sirupsen/logrus"
)

// DirectoryWatcher shares native registrations among directory cursors.
type DirectoryWatcher struct {
	// root confines directory lookup to the served filesystem.
	root *os.Root
	// le reports degraded refresh for this checkout root.
	le *logrus.Entry

	// mtx serializes native registrations and guards the registry.
	mtx sync.Mutex
	// native is nil when live refresh is unavailable or closed.
	native *directoryWatchSource
	// dirs owns each registration by the identity the native backend shares.
	dirs map[directoryWatchKey]*watchedDirectory
	// names routes native events to their registrations.
	names map[directoryWatchName]*watchedDirectory
	// warned records that loss of live refresh has been logged.
	warned bool
}

// watchedDirectory owns one registration and the open file that pins its identity.
type watchedDirectory struct {
	// key identifies the directory in the registry.
	key directoryWatchKey
	// name identifies the registration in native events.
	name directoryWatchName
	// dir pins the directory for the lifetime of the registration.
	dir *os.File
	// info identifies the pinned directory when rechecking cursor paths.
	info fs.FileInfo
	// listeners groups the open cursors by their path in the checkout root.
	listeners map[string]map[*func(bool)]struct{}
}

// NewDirectoryWatcher opens native refresh when the host has capacity.
func NewDirectoryWatcher(le *logrus.Entry, root *os.Root) *DirectoryWatcher {
	// Start one event source for all connections to the checkout root.
	native, err := openDirectoryWatchSource()
	w := &DirectoryWatcher{
		root: root, le: le, native: native,
		dirs:  make(map[directoryWatchKey]*watchedDirectory),
		names: make(map[directoryWatchName]*watchedDirectory),
	}
	if err != nil {
		w.warnLocked(err)
	}
	return w
}

// Watch subscribes a cursor until the returned function releases it.
// changed receives true when the cursor's path no longer names its directory.
// When native refresh is unavailable, the cursor can still read a snapshot.
func (w *DirectoryWatcher) Watch(path string, changed func(bool)) func() {
	// Report replacements after the registry lock is released.
	var retired []*func(bool)
	w.mtx.Lock()
	defer func() {
		w.mtx.Unlock()
		for _, cb := range retired {
			(*cb)(true)
		}
	}()

	// Keep the cursor usable if native refresh is unavailable.
	if w.native == nil {
		return func() {}
	}
	d, retired, err := w.openLocked(path)
	if err != nil {
		w.warnLocked(err)
		return func() {}
	}

	// Retain each cursor's path even when another path names the same inode.
	listeners := d.listeners[path]
	if listeners == nil {
		listeners = make(map[*func(bool)]struct{})
		d.listeners[path] = listeners
	}
	sub := &changed
	listeners[sub] = struct{}{}
	return func() { w.release(d, path, sub) }
}

// Run dispatches native events until the source or checkout root stops.
func (w *DirectoryWatcher) Run(ctx context.Context) {
	// Take the event source before it can be retired by Close.
	w.mtx.Lock()
	native := w.native
	w.mtx.Unlock()
	if native == nil {
		return
	}

	// Closing the source interrupts a blocked native read.
	stop := context.AfterFunc(ctx, func() { _ = w.Close() })
	defer stop()
	err := native.run(ctx, w)
	if err == nil {
		return
	}

	// Downgrade existing cursors to snapshots after a source failure.
	w.mtx.Lock()
	if w.native != native {
		w.mtx.Unlock()
		return
	}
	w.warnLocked(err)
	var listeners []*func(bool)
	for _, d := range w.dirs {
		listeners = appendDirectoryListeners(listeners, d)
	}
	_ = w.closeLocked()
	w.mtx.Unlock()
	for _, cb := range listeners {
		(*cb)(false)
	}
}

// Close ends native watches and releases the directories they pin.
func (w *DirectoryWatcher) Close() error {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.closeLocked()
}

// closeLocked clears the registry and stops its native source.
func (w *DirectoryWatcher) closeLocked() error {
	// Close the whole event source before dropping pinned directory handles.
	if w.native == nil {
		return nil
	}
	err := w.native.close()
	w.native = nil

	// Return the directory descriptors held by the retired registrations.
	for _, d := range w.dirs {
		err = errors.Join(err, d.dir.Close())
	}
	clear(w.dirs)
	clear(w.names)
	return err
}

// openLocked shares the existing registration or opens one for the current directory.
func (w *DirectoryWatcher) openLocked(path string) (*watchedDirectory, []*func(bool), error) {
	// Resolve the directory through the same confinement as the cursor.
	dir, err := w.root.Open(filepath.FromSlash(path))
	if err != nil {
		return nil, nil, err
	}
	info, err := dir.Stat()
	if err != nil {
		return nil, nil, errors.Join(err, dir.Close())
	}
	if !info.IsDir() {
		return nil, nil, errors.Join(fs.ErrInvalid, dir.Close())
	}
	key, err := directoryKey(w.root, path, info)
	if err != nil {
		return nil, nil, errors.Join(err, dir.Close())
	}

	// A path-based watch may still belong to the previous occupant of that path.
	var retired []*func(bool)
	if d := w.dirs[key]; d != nil {
		if os.SameFile(info, d.info) {
			_ = dir.Close()
			return d, nil, nil
		}
		retired = appendDirectoryListeners(retired, d)
		w.removeLocked(d)
	}

	// Only this registration may add or remove the native watch for its key.
	name, err := w.native.add(key, dir, info)
	if err != nil {
		return nil, retired, errors.Join(err, dir.Close())
	}
	d := &watchedDirectory{
		key: key, name: name, dir: dir, info: info,
		listeners: make(map[string]map[*func(bool)]struct{}),
	}
	w.dirs[key], w.names[name] = d, d
	return d, retired, nil
}

// release drops the kernel watch after its last cursor leaves.
func (w *DirectoryWatcher) release(d *watchedDirectory, path string, sub *func(bool)) {
	// Remove this cursor and discard its path after its last subscriber leaves.
	w.mtx.Lock()
	defer w.mtx.Unlock()
	listeners := d.listeners[path]
	delete(listeners, sub)
	if len(listeners) == 0 {
		delete(d.listeners, path)
	}

	// A queued release may belong to a registration that has already retired.
	if len(d.listeners) == 0 && w.dirs[d.key] == d {
		w.removeLocked(d)
	}
}

// removeLocked ends the registration and releases its pinned handle.
func (w *DirectoryWatcher) removeLocked(d *watchedDirectory) {
	delete(w.dirs, d.key)
	delete(w.names, d.name)
	w.native.remove(d.name)
	_ = d.dir.Close()
}

// dispatch wakes only the cursors using the native event's registration.
func (w *DirectoryWatcher) dispatch(name directoryWatchName, reconcile bool) {
	// Snapshot callbacks under the registry lock and invoke them outside it.
	w.mtx.Lock()
	d := w.names[name]
	if d == nil {
		w.mtx.Unlock()
		return
	}
	var changed, removed []*func(bool)
	switch {
	case reconcile:
		changed, removed = w.reconcileLocked(d)
	default:
		changed = appendDirectoryListeners(nil, d)
	}
	w.mtx.Unlock()

	// Invalidate surviving listings and release cursors whose paths moved.
	for _, cb := range changed {
		(*cb)(false)
	}
	for _, cb := range removed {
		(*cb)(true)
	}
}

// overflow reconciles all live registrations after the host drops events.
func (w *DirectoryWatcher) overflow() {
	// Let cursor opens proceed between checks of separate registrations.
	w.mtx.Lock()
	names := slices.Collect(maps.Keys(w.names))
	w.mtx.Unlock()

	// Reconcile each registration through the ordinary dispatch path.
	for _, name := range names {
		w.dispatch(name, true)
	}
}

// reconcileLocked retires moved cursors and rearms the shared watch for survivors.
func (w *DirectoryWatcher) reconcileLocked(d *watchedDirectory) (changed, removed []*func(bool)) {
	// Recheck a path once regardless of how many viewers opened it.
	for path, listeners := range d.listeners {
		info, err := w.root.Stat(filepath.FromSlash(path))
		if errors.Is(err, fs.ErrNotExist) || (err == nil && !os.SameFile(info, d.info)) {
			delete(d.listeners, path)
			removed = slices.AppendSeq(removed, maps.Keys(listeners))
			continue
		}
		if err != nil {
			w.warnLocked(err)
			delete(d.listeners, path)
		}
		changed = slices.AppendSeq(changed, maps.Keys(listeners))
	}

	// The native registration follows the directory only while a cursor needs it.
	if len(d.listeners) == 0 {
		w.removeLocked(d)
		return changed, removed
	}
	name, err := w.native.add(d.key, d.dir, d.info)
	if err != nil {
		w.warnLocked(err)
		w.removeLocked(d)
		return changed, removed
	}
	delete(w.names, d.name)
	d.name = name
	w.names[name] = d
	return changed, removed
}

// appendDirectoryListeners collects callbacks that will run outside the registry lock.
func appendDirectoryListeners(out []*func(bool), d *watchedDirectory) []*func(bool) {
	for _, listeners := range d.listeners {
		out = slices.AppendSeq(out, maps.Keys(listeners))
	}
	return out
}

// warnLocked logs the first refresh failure for this checkout root.
func (w *DirectoryWatcher) warnLocked(err error) {
	if !w.warned {
		w.warned = true
		w.le.WithError(err).Warn("checkout-root live directory refresh unavailable")
	}
}
