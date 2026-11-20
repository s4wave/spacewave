// Package core_unixfs_linkedfolder runs linked folders: each keeps one Drive
// equal to a folder on this Device, with the files' blocks read from the
// files.
package core_unixfs_linkedfolder

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/fsnotify"
	"github.com/pkg/errors"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	unixfs_linkedfolder "github.com/s4wave/spacewave/sdk/unixfs/linkedfolder"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "unixfs/linked-folder"

// FileSegmentEngine is a World engine whose Device serves blocks from linked
// files: the Space World engine of a Device whose store links files.
type FileSegmentEngine interface {
	// GetFileSegments returns the store serving the blocks of linked files,
	// or nil when the Device's store cannot link files.
	GetFileSegments() *block_store_filesegment.Store
}

// watchQuiet is how long the folder stays quiet after a change before the
// changed directories sync, so a burst of writes syncs once.
const watchQuiet = 500 * time.Millisecond

// watchMaxWait bounds how long a change waits for its sync while the folder
// keeps changing.
const watchMaxWait = 5 * time.Second

// Controller keeps one Drive equal to a folder on this Device. On start it
// watches the folder, then syncs it with a stat walk, linking only what
// changed since the Drive last matched it. After that it syncs each directory
// the watcher names, and the whole folder when the watcher dropped events.
// While the folder's root is off the file system it was linked on, as when
// its disk is unmounted, it syncs nothing and waits for the file system.
type Controller struct {
	// le is the logger.
	le *logrus.Entry
	// conf is the config.
	conf *unixfs_linkedfolder.Config
	// engine is the World engine holding the Drive.
	engine *world.BusEngine
}

// NewController constructs a linked-folder controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *unixfs_linkedfolder.Config) *Controller {
	return &Controller{
		le:     le,
		conf:   conf,
		engine: world.NewBusEngine(nil, b, conf.GetEngineId()),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "linked folder controller")
}

// Execute syncs the folder into the Drive, then follows its changes until the
// context ends. A change of the engine serving the World syncs again.
func (c *Controller) Execute(ctx context.Context) error {
	c.engine.SetContext(ctx)
	return world.AccessEngine(ctx, c.engine, func(ctx context.Context, eng world.Engine) error {
		// Require a store that serves linked files.
		fe, ok := eng.(FileSegmentEngine)
		if !ok {
			return errors.New("the World cannot link files")
		}
		files := fe.GetFileSegments()
		if files == nil {
			return errors.New("this Device's store cannot link files")
		}

		// Follow the folder while its root is on the file system it was
		// linked on, and wait for that file system while it is not.
		link, err := unixfs_world.NewFolderLink(eng, files.GetIndex(), c.conf.GetObjectKey(), c.conf.GetFolder(), c.conf.DestPath())
		if err != nil {
			return err
		}
		for {
			err := c.run(ctx, link)
			if !errors.Is(err, unixfs_world.ErrFolderRootChanged) {
				return err
			}
			c.le.WithError(err).Warn("waiting for the folder's file system")
			if err := waitRoot(ctx, link); err != nil {
				return err
			}
		}
	})
}

// rootRecheck is how often a link waiting for its folder's file system
// checks the root again. No portable event announces a mount, and a check
// costs one stat and one index read.
const rootRecheck = 5 * time.Second

// waitRoot returns once the folder's root is back on the file system it was
// linked on.
func waitRoot(ctx context.Context, link *unixfs_world.FolderLink) error {
	ticker := time.NewTicker(rootRecheck) //gostyle:timer external
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-ticker.C:
		}
		err := link.CheckRoot(ctx)
		if !errors.Is(err, unixfs_world.ErrFolderRootChanged) {
			return err
		}
	}
}

// run syncs the folder and follows its changes. It returns
// ErrFolderRootChanged once the root leaves its file system.
func (c *Controller) run(ctx context.Context, link *unixfs_world.FolderLink) error {
	// Watch the folder before the sync reads it, so no change escapes both.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	folder := filepath.Clean(c.conf.GetFolder())
	if err := watcher.Add(filepath.Join(folder, "...")); err != nil {
		return errors.Wrap(err, "watch folder")
	}

	// Sync the folder, then follow its changes.
	if err := link.Sync(ctx, c.logProgress); err != nil {
		return err
	}
	c.le.Debug("folder synced")
	return c.follow(ctx, link, watcher, folder)
}

// follow syncs the directories the watcher names once the folder has been
// quiet for watchQuiet, or watchMaxWait after the first unsynced change. An
// event outside the folder's tree, on the folder itself, or a report of
// dropped events syncs the whole folder.
func (c *Controller) follow(ctx context.Context, link *unixfs_world.FolderLink, watcher *fsnotify.Watcher, folder string) error {
	// Track the changes not yet synced and the timers that end their wait.
	dirs := make(map[string]struct{})
	var all bool
	var quiet, deadline *time.Timer
	var quietC, deadlineC <-chan time.Time
	for {
		// Collect changes until the folder is quiet or the deadline passes.
		changed := false
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case ev, ok := <-watcher.Events:
			if !ok {
				return errors.New("folder watcher closed")
			}
			rel, err := filepath.Rel(folder, ev.Name)
			if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				all = true
			} else {
				dirs[filepath.Dir(rel)] = struct{}{}
			}
			changed = true
		case err, ok := <-watcher.Errors:
			if !ok {
				return errors.New("folder watcher closed")
			}
			if !errors.Is(err, fsnotify.ErrEventOverflow) {
				return errors.Wrap(err, "watch folder")
			}
			c.le.WithError(err).Debug("folder watcher dropped events, syncing the whole folder")
			all, changed = true, true
		case <-quietC:
		case <-deadlineC:
		}
		if changed {
			if quiet == nil {
				quiet, deadline = time.NewTimer(watchQuiet), time.NewTimer(watchMaxWait)
				quietC, deadlineC = quiet.C, deadline.C
			} else {
				quiet.Reset(watchQuiet)
			}
			continue
		}

		// Sync what changed.
		quiet.Stop()
		deadline.Stop()
		quiet, deadline, quietC, deadlineC = nil, nil, nil, nil
		var err error
		if all {
			err = link.Sync(ctx, c.logProgress)
		} else {
			err = link.SyncDirs(ctx, slices.Collect(maps.Keys(dirs)), c.logProgress)
		}
		if err != nil {
			return err
		}
		clear(dirs)
		all = false
	}
}

// logProgress logs the files a sync linked so far.
func (c *Controller) logProgress(files, bytes uint64) error {
	c.le.WithField("files", files).WithField("bytes", bytes).Debug("linked files")
	return nil
}

// HandleDirective resolves no directives.
func (c *Controller) HandleDirective(context.Context, directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
