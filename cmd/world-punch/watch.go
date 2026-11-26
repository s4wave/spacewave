package main

import (
	"path/filepath"
	"sync"

	"github.com/aperturerobotics/fsnotify"
	"github.com/pkg/errors"
)

// fileWatch reports each change to one file. It watches the file's directory,
// so a file created, replaced or removed after the watch starts is seen too.
type fileWatch struct {
	w    *fsnotify.Watcher
	path string

	mu sync.Mutex
	// ch is closed on the next change and then replaced.
	ch chan struct{}
}

// watchFile starts a fileWatch on path. Close it when done.
func watchFile(path string) (*fileWatch, error) {
	// Watch the directory holding path.
	path = filepath.Clean(path)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.Wrap(err, "watch")
	}
	if err := w.Add(filepath.Dir(path)); err != nil {
		_ = w.Close()
		return nil, errors.Wrapf(err, "watch %s", filepath.Dir(path))
	}

	// Signal each event on path until the watcher closes.
	fw := &fileWatch{w: w, path: path, ch: make(chan struct{})}
	go fw.run()
	return fw, nil
}

// run signals each event on the file. A watcher error may mean lost events,
// so it signals too.
func (fw *fileWatch) run() {
	for {
		select {
		case ev, ok := <-fw.w.Events:
			if !ok {
				return
			}
			if filepath.Clean(ev.Name) == fw.path {
				fw.signal()
			}
		case _, ok := <-fw.w.Errors:
			if !ok {
				return
			}
			fw.signal()
		}
	}
}

// signal wakes every waiter on the current channel.
func (fw *fileWatch) signal() {
	fw.mu.Lock()
	close(fw.ch)
	fw.ch = make(chan struct{})
	fw.mu.Unlock()
}

// changed returns a channel closed on the next change after the call. Take it
// before reading the file, so a change during the read is not missed. A nil
// watch returns nil, which never fires.
func (fw *fileWatch) changed() <-chan struct{} {
	if fw == nil {
		return nil
	}
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.ch
}

// Close stops the watch.
func (fw *fileWatch) Close() error {
	return fw.w.Close()
}
