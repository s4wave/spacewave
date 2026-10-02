//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"path/filepath"
	"sync"

	"github.com/aperturerobotics/fsnotify"
)

// sharedDirWatcher watches the directories of every executing Store in the
// process. Each fsnotify watcher holds a Linux inotify instance, and the kernel
// limits instances per user (128 by default), so a watcher per Store fails
// once the user's processes open that many stores.
var sharedDirWatcher dirWatcher

// dirWatcher shares one fsnotify watcher among subscriptions and reference
// counts the directories they watch. The watcher starts with the first
// subscription and closes with the last.
type dirWatcher struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// watcher is the shared watcher, nil while no subscription is active.
	watcher *fsnotify.Watcher
	// dirs counts the subscriptions watching each directory.
	dirs map[string]int
	// subs is the set of active subscriptions.
	subs map[*dirWatch]struct{}
}

// dirWatch is one subscription to entry changes in a directory.
type dirWatch struct {
	dir string
	// changed receives a value after an entry in dir, or dir itself, is
	// created, removed, or renamed.
	changed chan struct{}
	// errs receives errors from the shared watcher.
	errs chan error
}

// watch subscribes to entry changes in dir. Release the subscription with
// release.
func (w *dirWatcher) watch(dir string) (*dirWatch, error) {
	// Hold the lock for the whole subscription.
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Start the shared watcher with the first subscription.
	if w.watcher == nil {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, err
		}
		w.watcher = watcher
		w.dirs = make(map[string]int)
		w.subs = make(map[*dirWatch]struct{})
		go w.dispatch(watcher)
	}

	// Add dir to the watcher unless another subscription already watches it.
	if w.dirs[dir] == 0 {
		if err := w.watcher.Add(dir); err != nil {
			w.closeIdleLocked()
			return nil, err
		}
	}
	w.dirs[dir]++

	// Register the subscription for dispatch.
	sub := &dirWatch{
		dir:     dir,
		changed: make(chan struct{}, 1),
		errs:    make(chan error, 1),
	}
	w.subs[sub] = struct{}{}
	return sub, nil
}

// release ends a subscription returned by watch.
func (w *dirWatcher) release(sub *dirWatch) {
	// Hold the lock for the whole release.
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// Stop watching dir with its last subscription. The kernel drops the
	// watch itself when dir is removed, so Remove may find none.
	delete(w.subs, sub)
	w.dirs[sub.dir]--
	if w.dirs[sub.dir] == 0 {
		delete(w.dirs, sub.dir)
		_ = w.watcher.Remove(sub.dir)
	}

	// Close the watcher with the last subscription.
	w.closeIdleLocked()
}

// closeIdleLocked closes the shared watcher when no subscription remains.
func (w *dirWatcher) closeIdleLocked() {
	if len(w.subs) != 0 {
		return
	}
	_ = w.watcher.Close()
	w.watcher, w.dirs, w.subs = nil, nil, nil
}

// dispatch forwards events and errors from watcher to the subscriptions until
// watcher closes.
func (w *dirWatcher) dispatch(watcher *fsnotify.Watcher) {
	for {
		select {
		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Every commit writes the database file; only creating, removing,
			// or renaming an entry can change what a Store's paths refer to.
			if !ev.Has(fsnotify.Create | fsnotify.Remove | fsnotify.Rename) {
				continue
			}
			w.notify(watcher, func(sub *dirWatch) {
				if sub.dir != ev.Name && sub.dir != filepath.Dir(ev.Name) {
					return
				}
				select {
				case sub.changed <- struct{}{}:
				default:
				}
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			w.notify(watcher, func(sub *dirWatch) {
				select {
				case sub.errs <- err:
				default:
				}
			})
		}
	}
}

// notify calls fn for each subscription while watcher is still the shared
// watcher.
func (w *dirWatcher) notify(watcher *fsnotify.Watcher, fn func(sub *dirWatch)) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.watcher != watcher {
		return
	}
	for sub := range w.subs {
		fn(sub)
	}
}
