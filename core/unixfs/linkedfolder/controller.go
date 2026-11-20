// Package core_unixfs_linkedfolder runs linked folders: each keeps one Drive
// equal to a folder on this Device, with the files' blocks read from the
// files.
package core_unixfs_linkedfolder

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/fsnotify"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
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
//
// Its node status shows indexing during a full sync, watching while it
// follows the folder, with the files waiting out a backoff because they kept
// changing during their reads, and rejected, naming both file systems, while
// the folder's root is off its file system.
type Controller struct {
	// le is the logger.
	le *logrus.Entry
	// conf is the config.
	conf *unixfs_linkedfolder.Config
	// engine is the World engine holding the Drive.
	engine *world.BusEngine

	// status guards state and detail and wakes their watchers.
	status broadcast.Broadcast
	// state is the node's state.
	state s4wave_device.DeviceCapabilityState
	// detail describes state.
	detail string

	// progress guards files and bytes and wakes their watchers. It is apart
	// from status because the node status reaches the Device record, and
	// the counts change after every committed batch.
	progress broadcast.Broadcast
	// files and bytes count what the current or last full sync linked.
	files, bytes uint64
}

// NewController constructs a linked-folder controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *unixfs_linkedfolder.Config) *Controller {
	return &Controller{
		le:     le,
		conf:   conf,
		engine: world.NewBusEngine(nil, b, conf.GetEngineId()),
		state:  s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE,
		detail: unixfs_linkedfolder.StatusStarting,
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
		link, err := unixfs_world.NewFolderLink(eng, files.GetIndex(), c.conf.GetObjectKey(), c.conf.GetFolder(), c.conf.DestPath(), c.conf.GetTwoWay())
		if err != nil {
			return err
		}
		stale := newStalePaths()
		release := files.WatchHashMisses(stale.add)
		defer release()
		remote := make(chan struct{}, 1)
		if c.conf.GetTwoWay() {
			go c.watchDrive(ctx, eng, remote)
		}
		for {
			err := c.run(ctx, link, stale, remote)
			if !errors.Is(err, unixfs_world.ErrFolderRootChanged) {
				return err
			}
			c.le.WithError(err).Warn("waiting for the folder's file system")
			c.setStatus(s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_REJECTED, err.Error())
			if err := waitRoot(ctx, link); err != nil {
				return err
			}
		}
	})
}

// watchDrive signals remote, without blocking, each time the Drive's object
// changes, so a two-way link applies others' changes as they commit. Each
// signal costs the follow loop one remote pass after its quiet period.
func (c *Controller) watchDrive(ctx context.Context, eng world.Engine, remote chan<- struct{}) {
	// Wait for each revision after the current one.
	ws := world.NewEngineWorldState(eng, false)
	key := c.conf.GetObjectKey()
	rev, _, err := world.GetObjectRev(ctx, ws, key)
	for err == nil {
		rev, err = world.WaitObjectRev(ctx, ws, key, rev+1, true)
		if err == nil {
			select {
			case remote <- struct{}{}:
			default:
			}
		}
	}

	// Report a watch that ended before the controller did.
	if ctx.Err() == nil {
		c.le.WithError(err).Warn("stopped watching the Drive for others' changes")
	}
}

// unappliedRetry is how long a two-way link waits before trying again the
// remote changes a pass could not apply, as when a member's blocks were not
// reachable yet.
const unappliedRetry = 10 * time.Second

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
func (c *Controller) run(ctx context.Context, link *unixfs_world.FolderLink, stale *stalePaths, remote <-chan struct{}) error {
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
	c.showIndexing()
	err = link.Sync(ctx, c.logProgress)
	if err != nil && !isFileConflict(err) {
		return err
	}
	c.le.Debug("folder synced")
	c.showWatching(link)
	return c.follow(ctx, link, watcher, stale, remote, folder, err != nil)
}

// follow syncs the directories the watcher names once the folder has been
// quiet for watchQuiet, or watchMaxWait after the first unsynced change. An
// event outside the folder's tree, on the folder itself, or a report of
// dropped events syncs the whole folder. A file whose bytes the store found
// changed under an unchanged stat is linked again with the same wait, and so
// is a file that kept changing during its reads once its backoff passes.
//
// A two-way link also syncs, with the same wait, after each change others
// commit to the Drive, and after unappliedRetry while a pass left remote
// changes unapplied. A sync refused because another writer changed its files
// first syncs the whole folder again, applying that change; so does the
// first one when resync is set.
func (c *Controller) follow(
	ctx context.Context,
	link *unixfs_world.FolderLink,
	watcher *fsnotify.Watcher,
	stale *stalePaths,
	remote <-chan struct{},
	folder string,
	resync bool,
) error {
	// Track the changes not yet synced and the timers that end their wait.
	dirs := make(map[string]struct{})
	files := make(map[string]struct{})
	all := resync
	var applyRemote bool

	// Wake to retry the remote changes the last pass left unapplied.
	var retry *time.Timer
	var retryC <-chan time.Time
	defer func() {
		if retry != nil {
			retry.Stop()
		}
	}()
	armRetry := func() {
		if retry != nil {
			retry.Stop()
		}
		retry, retryC = nil, nil
		if n, err := link.Unapplied(); n != 0 {
			c.le.WithError(err).WithField("changes", n).Warn("others' changes left to apply")
			retry = time.NewTimer(unappliedRetry)
			retryC = retry.C
		}
	}
	armRetry()
	var quiet, deadline *time.Timer
	var quietC, deadlineC <-chan time.Time

	// Wake when the earliest backoff of a changing file passes.
	var changing *time.Timer
	var changingC <-chan time.Time
	defer func() {
		if changing != nil {
			changing.Stop()
		}
	}()
	armChanging := func() {
		if changing != nil {
			changing.Stop()
		}
		changing, changingC = nil, nil

		// Arm it for the earliest due file, if any.
		if n, due := link.Changing(); n != 0 {
			changing = time.NewTimer(time.Until(due))
			changingC = changing.C
		}
	}

	// Arm it for the files the starting sync left changing.
	armChanging()

	// Sync each batch of changes until the context ends.
	for {
		// Collect changes until the folder is quiet or the deadline passes.
		// A wake that names nothing in this folder, such as a hash miss in
		// another folder on the same store, changes nothing and arms no timer.
		var changed, due bool
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case ev, ok := <-watcher.Events:
			if !ok {
				return errors.New("folder watcher closed")
			}
			if rel, ok := relPath(folder, ev.Name); ok {
				dirs[filepath.Dir(rel)] = struct{}{}
			} else {
				all = true
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
		case <-stale.wake:
			for _, path := range stale.take() {
				if rel, ok := relPath(folder, path); ok {
					files[rel] = struct{}{}
					changed = true
				}
			}
		case <-changingC:
			changingC = nil
			for _, rel := range link.DueChanging(time.Now()) {
				files[rel] = struct{}{}
			}
			changed = true
		case <-remote:
			applyRemote, changed = true, true
		case <-retryC:
			retryC = nil
			applyRemote, changed = true, true
		case <-quietC:
			due = true
		case <-deadlineC:
			due = true
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
		if !due {
			continue
		}

		// Sync what changed.
		quiet.Stop()
		deadline.Stop()
		quiet, deadline, quietC, deadlineC = nil, nil, nil, nil
		var err error
		switch {
		case all:
			c.showIndexing()
			err = link.Sync(ctx, c.logProgress)
		case len(dirs) != 0:
			err = link.SyncDirs(ctx, slices.Collect(maps.Keys(dirs)), c.logProgress)
		case applyRemote && len(files) == 0:
			// Only the Drive changed: apply its changes, and let the
			// watcher name what they wrote.
			err = link.ApplyRemote(ctx)
		}
		if err == nil && len(files) != 0 {
			err = link.Reindex(ctx, slices.Collect(maps.Keys(files)), c.logProgress)
		}
		conflict := isFileConflict(err)
		if err != nil && !conflict {
			return err
		}
		clear(dirs)
		clear(files)
		all, applyRemote = conflict, false
		c.showWatching(link)
		armChanging()
		armRetry()
		if conflict {
			// Sync again after the quiet period, applying the change first.
			c.le.WithError(err).Debug("another writer changed the Drive first, syncing again")
			quiet, deadline = time.NewTimer(watchQuiet), time.NewTimer(watchMaxWait)
			quietC, deadlineC = quiet.C, deadline.C
		}
	}
}

// isFileConflict reports whether err is a two-way sync refused because
// another writer changed its files first.
func isFileConflict(err error) bool {
	var conflict *unixfs_world.FileConflictError
	return errors.As(err, &conflict)
}

// relPath returns path relative to folder, when it lies inside it and is not
// the folder itself.
func relPath(folder, path string) (string, bool) {
	rel, err := filepath.Rel(folder, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// stalePaths collects the paths of files whose bytes the store found changed
// under an unchanged stat, as its reads report them.
type stalePaths struct {
	// mtx guards paths.
	mtx sync.Mutex
	// paths holds the reported paths not yet taken.
	paths map[string]struct{}
	// wake holds a signal while paths is not empty.
	wake chan struct{}
}

// newStalePaths constructs an empty stalePaths.
func newStalePaths() *stalePaths {
	return &stalePaths{paths: make(map[string]struct{}), wake: make(chan struct{}, 1)}
}

// add records path and signals wake; it never blocks the read reporting it.
func (p *stalePaths) add(path string) {
	p.mtx.Lock()
	p.paths[path] = struct{}{}
	p.mtx.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// take returns and clears the recorded paths.
func (p *stalePaths) take() []string {
	// Swap out the recorded paths under the lock.
	p.mtx.Lock()
	defer p.mtx.Unlock()
	out := slices.Collect(maps.Keys(p.paths))
	clear(p.paths)
	return out
}

// GetNodeStatus returns the node's state and detail, and a channel closed at
// their next change.
func (c *Controller) GetNodeStatus() (s4wave_device.DeviceCapabilityState, string, <-chan struct{}) {
	l := c.status.Lock()
	defer l.Unlock()
	return c.state, c.detail, l.WaitCh()
}

// setStatus shows state and detail, waking the watchers when they change.
func (c *Controller) setStatus(state s4wave_device.DeviceCapabilityState, detail string) {
	// Keep an unchanged status quiet.
	l := c.status.Lock()
	defer l.Unlock()
	if c.state == state && c.detail == detail {
		return
	}

	// Show the new status.
	c.state, c.detail = state, detail
	l.Broadcast()
}

// showIndexing shows a full sync and starts its counts again. A targeted sync
// keeps showing watching, so following the folder writes the Device only when
// the status changes.
func (c *Controller) showIndexing() {
	c.setProgress(0, 0)
	c.setStatus(s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE, unixfs_linkedfolder.StatusIndexing)
}

// showWatching shows the link following the folder, with the files waiting
// out a backoff because they kept changing during their reads.
func (c *Controller) showWatching(link *unixfs_world.FolderLink) {
	detail := unixfs_linkedfolder.StatusWatching
	switch n, _ := link.Changing(); n {
	case 0:
	case 1:
		detail += "; 1 file changing, will link when quiet"
	default:
		detail += fmt.Sprintf("; %d files changing, will link when quiet", n)
	}
	c.setStatus(s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE, detail)
}

// logProgress shows the files a sync linked so far. Only a full sync's counts
// are shown: a targeted sync counts from zero again, and the counts of the full
// sync before it stay.
func (c *Controller) logProgress(files, bytes uint64) error {
	c.le.WithField("files", files).WithField("bytes", bytes).Debug("linked files")
	if c.isIndexing() {
		c.setProgress(files, bytes)
	}
	return nil
}

// isIndexing reports whether a full sync runs.
func (c *Controller) isIndexing() bool {
	l := c.status.Lock()
	defer l.Unlock()
	return c.detail == unixfs_linkedfolder.StatusIndexing
}

// setProgress shows the counts, waking their watchers when they change.
func (c *Controller) setProgress(files, bytes uint64) {
	// Keep unchanged counts quiet.
	l := c.progress.Lock()
	defer l.Unlock()
	if c.files == files && c.bytes == bytes {
		return
	}

	// Show the new counts.
	c.files, c.bytes = files, bytes
	l.Broadcast()
}

// GetConfig returns the link's config.
func (c *Controller) GetConfig() *unixfs_linkedfolder.Config {
	return c.conf
}

// GetLinkProgress returns the counts of the current or last full sync, and a
// channel closed at their next change.
func (c *Controller) GetLinkProgress() (unixfs_linkedfolder.LinkProgress, <-chan struct{}) {
	l := c.progress.Lock()
	defer l.Unlock()
	return unixfs_linkedfolder.LinkProgress{Files: c.files, Bytes: c.bytes}, l.WaitCh()
}

// HandleDirective resolves a lookup of the linked folders writing its Drive.
func (c *Controller) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	if dir, ok := di.GetDirective().(unixfs_linkedfolder.LookupLinkedFolder); ok {
		return unixfs_linkedfolder.ResolveLinkedFolder(dir, c)
	}
	return nil, nil
}

// Close releases the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller                = (*Controller)(nil)
	_ s4wave_flowgraph.FlowgraphNodeStatus = (*Controller)(nil)
	_ unixfs_linkedfolder.RunningLink      = (*Controller)(nil)
)
