//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aperturerobotics/fsnotify"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
)

// SourceWatcher watches the directories of a builder's input files for the
// lifetime of the builder, so an edit made while a build runs triggers the
// next build instead of being lost between watchers.
//
// It watches directories rather than files: editors replace files atomically,
// dropping the watch on the original file. fsnotify needs one watch per
// directory: https://github.com/fsnotify/fsnotify/issues/18
//
// A DirSuffix input is a directory, watched itself, and a matching file
// created, removed or renamed in it changes that input.
type SourceWatcher struct {
	// watcher is the fsnotify watcher, closed when the watch context ends.
	watcher *fsnotify.Watcher
	// dirs is the set of watched directories, owned by the SetPaths caller.
	dirs map[string]struct{}

	// bcast guards the fields below.
	bcast broadcast.Broadcast
	// paths maps each watched source path to its input file.
	paths map[string]*bldr_manifest_builder.InputManifest_File
	// changed is the set of paths changed since the last take.
	changed map[string]struct{}
	// listed is the set of paths created, removed or renamed since the last
	// take, which change the DirSuffix input of their directory.
	listed map[string]struct{}
	// events counts the events recorded since the last take.
	events int
	// lastEvent is when the last change was recorded.
	lastEvent time.Time
	// takenAt is when the changes were last taken.
	takenAt time.Time
	// err is the terminal watcher error.
	err error
}

// NewSourceWatcher constructs a SourceWatcher that reads events until ctx ends.
func NewSourceWatcher(ctx context.Context) (*SourceWatcher, error) {
	// Create the watcher with no directories. SetPaths adds them.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Read events for the lifetime of ctx.
	w := &SourceWatcher{
		watcher: watcher,
		dirs:    make(map[string]struct{}),
		changed: make(map[string]struct{}),
		listed:  make(map[string]struct{}),
		takenAt: time.Now(),
	}
	go w.readEvents(ctx)
	return w, nil
}

// SetPaths replaces the watched source paths, watching each path's directory,
// or the path itself for a DirSuffix input. Changes recorded before the call
// stay pending and are filtered against the new paths when taken.
func (w *SourceWatcher) SetPaths(paths map[string]*bldr_manifest_builder.InputManifest_File) error {
	// Collect the directories the new paths need.
	dirs := make(map[string]struct{}, len(paths))
	for sourcePath, file := range paths {
		if file.GetDirSuffix() != "" {
			dirs[sourcePath] = struct{}{}
			continue
		}
		dirs[filepath.Dir(sourcePath)] = struct{}{}
	}

	// Watch the new directories and drop the unused ones.
	for dir := range dirs {
		if _, ok := w.dirs[dir]; ok {
			continue
		}
		if err := w.watcher.Add(dir); err != nil {
			return errors.Wrapf(err, "watch %s", dir)
		}
		w.dirs[dir] = struct{}{}
	}
	for dir := range w.dirs {
		if _, ok := dirs[dir]; ok {
			continue
		}
		_ = w.watcher.Remove(dir)
		delete(w.dirs, dir)
	}

	// Publish the new paths.
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		w.paths = paths
		broadcast()
	})
	return nil
}

// GetDirCount returns the number of watched directories.
func (w *SourceWatcher) GetDirCount() int {
	return len(w.dirs)
}

// Wait waits for at least one watched path to change, then for quiet to pass
// with no further changes. It returns the changed input files and the number
// of events behind them, and clears the pending changes.
func (w *SourceWatcher) Wait(
	ctx context.Context,
	quiet time.Duration,
) ([]*bldr_manifest_builder.InputManifest_File, int, error) {
	for {
		// Take the changes once they have been quiet long enough.
		var files []*bldr_manifest_builder.InputManifest_File
		var events int
		var remaining time.Duration
		var err error
		var waitCh <-chan struct{}
		w.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			// Read the state under the lock, then take settled changes.
			waitCh = getWaitCh()
			err = w.err
			if err != nil || len(w.changed) == 0 {
				return
			}
			if since := time.Since(w.lastEvent); since < quiet {
				remaining = quiet - since
				return
			}
			files, events = w.takeChanges()
		})
		if err != nil {
			return nil, 0, err
		}
		if len(files) != 0 {
			return files, events, nil
		}

		// Wait for the next change, or for the quiet period to end.
		var timer *time.Timer
		var timerCh <-chan time.Time
		if remaining > 0 {
			timer = time.NewTimer(remaining)
			timerCh = timer.C
		}
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-waitCh:
		case <-timerCh:
		}
		if timer != nil {
			timer.Stop()
		}
		if err != nil {
			return nil, 0, err
		}
	}
}

// takeChanges returns the changed input files and clears the pending changes.
// Changed paths that are not watched input files are dropped.
// Caller holds the bcast lock.
func (w *SourceWatcher) takeChanges() ([]*bldr_manifest_builder.InputManifest_File, int) {
	// Map the changed paths to their input files, once each.
	var files []*bldr_manifest_builder.InputManifest_File
	seen := make(map[*bldr_manifest_builder.InputManifest_File]struct{}, len(w.changed))
	take := func(file *bldr_manifest_builder.InputManifest_File) {
		if file == nil {
			return
		}
		if _, ok := seen[file]; ok {
			return
		}
		seen[file] = struct{}{}
		files = append(files, file)
	}
	for sourcePath := range w.changed {
		// A DirSuffix directory's own write reports an entry change
		// that the listed paths below decide. Only the directory's
		// creation or removal counts.
		file := w.paths[sourcePath]
		if _, listed := w.listed[sourcePath]; file.GetDirSuffix() != "" && !listed {
			continue
		}
		take(file)
	}

	// Map each listed path to the DirSuffix input of its directory.
	for sourcePath := range w.listed {
		dir := w.paths[filepath.Dir(sourcePath)]
		if suffix := dir.GetDirSuffix(); suffix != "" && strings.HasSuffix(sourcePath, suffix) {
			take(dir)
		}
	}

	// Clear the pending changes.
	events := w.events
	clear(w.changed)
	clear(w.listed)
	w.events = 0
	w.takenAt = time.Now()
	return files, events
}

// readEvents records watcher events until ctx ends, then closes the watcher.
func (w *SourceWatcher) readEvents(ctx context.Context) {
	// Record each event, stopping at the first watcher error.
	defer w.watcher.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.recordEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				w.err = errors.Wrap(err, "watcher error")
				broadcast()
			})
			return
		}
	}
}

// recordEvent records one watcher event as a pending change.
//
// Content and name changes always count. A Chmod alone counts only when the
// file's modification time moved past the last take. That Chmod is how touch
// reports itself on kqueue, while a permission change by a build does not.
func (w *SourceWatcher) recordEvent(event fsnotify.Event) {
	// Classify the event, reading the modification time for a bare Chmod.
	contentOps := fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename
	var modTime time.Time
	if event.Op&contentOps == 0 {
		if !event.Has(fsnotify.Chmod) {
			return
		}
		info, err := os.Stat(event.Name)
		if err != nil {
			return
		}
		modTime = info.ModTime()
	}

	// Record the change and wake the waiter.
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Skip a chmod that predates the last take.
		if !modTime.IsZero() && !modTime.After(w.takenAt) {
			return
		}

		// Mark the path, and its directory listing when an entry changed.
		w.changed[event.Name] = struct{}{}
		if event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
			w.listed[event.Name] = struct{}{}
		}

		// Count the event and wake the waiter.
		w.events++
		w.lastEvent = time.Now()
		broadcast()
	})
}
