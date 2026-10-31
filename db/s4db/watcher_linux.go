package s4db

import (
	"os"
)

// fileWatcher waits for writes to the database file with a watch on the
// process's inotify instance.
type fileWatcher struct {
	// wd is the watch descriptor of the file.
	wd int
	// changed holds one pending change.
	changed chan struct{}
	// done is closed by stop.
	done chan struct{}
}

// newFileWatcher watches the file at path. Every write raises an inotify event,
// so the slot is unused.
func newFileWatcher(_ *os.File, path string, _ int) (*fileWatcher, error) {
	w := &fileWatcher{changed: make(chan struct{}, 1), done: make(chan struct{})}
	if err := processInotify.add(path, w); err != nil {
		return nil, err
	}
	return w, nil
}

// wait blocks until the file changes, reporting false once stopped.
func (w *fileWatcher) wait() bool {
	select {
	case <-w.done:
		return false
	case <-w.changed:
	}

	// A stop takes precedence over a change that arrived with it.
	select {
	case <-w.done:
		return false
	default:
		return true
	}
}

// wake records a change for wait. One pending change serves any number of
// writes, since one tail applies them all.
func (w *fileWatcher) wake() {
	select {
	case w.changed <- struct{}{}:
	default:
	}
}

// notify tells other processes the file changed. The kernel already did.
func (w *fileWatcher) notify() {}

// stop wakes wait for the last time.
func (w *fileWatcher) stop() {
	close(w.done)
}

// close releases the watch.
func (w *fileWatcher) close() {
	processInotify.remove(w)
}

// _ is a type assertion
var _ watcher = (*fileWatcher)(nil)
