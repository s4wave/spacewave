//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/fsnotify"
	backoff "github.com/aperturerobotics/util/backoff/cbackoff"
	debounce_fswatcher "github.com/aperturerobotics/util/debounce-fswatcher"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/s4wave/spacewave/bldr/manifest/builder/resultworld"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bldr/manifest/builder/controller"

// Controller is the builder controller.
type Controller struct {
	// le is the controller logger.
	le *logrus.Entry
	// bus loads the configured manifest builder.
	bus bus.Bus
	// c selects the manifest builder configuration.
	c *Config
	// pluginBuildLimiter bounds whole-plugin build attempts.
	pluginBuildLimiter *PluginBuildLimiter
	// resultPromise reports the current build result.
	resultPromise *promise.PromiseContainer[*bldr_manifest_builder.BuilderResult]
	// subManifestBuilderTrackers manage watched sub-manifest builders.
	subManifestBuilderTrackers *keyed.Keyed[string, *subManifestBuilderTracker]

	// mtx guards lifecycleSink and lifecycleStatus.
	mtx sync.Mutex
	// lifecycleSink receives the latest lifecycle status.
	lifecycleSink ManifestBuilderLifecycleSink
	// lifecycleStatus is the latest lifecycle status.
	lifecycleStatus ManifestBuilderLifecycleStatus
}

// NewController constructs a new controller.
func NewController(le *logrus.Entry, bus bus.Bus, cc *Config) *Controller {
	return newController(le, bus, cc, NewPluginBuildLimiter(0))
}

func newController(
	le *logrus.Entry,
	bus bus.Bus,
	cc *Config,
	pluginBuildLimiter *PluginBuildLimiter,
) *Controller {
	c := &Controller{
		le:                 le,
		bus:                bus,
		c:                  cc,
		pluginBuildLimiter: pluginBuildLimiter,
		resultPromise:      promise.NewPromiseContainer[*bldr_manifest_builder.BuilderResult](),
	}
	c.subManifestBuilderTrackers = keyed.NewKeyedWithLogger(c.newSubManifestBuilderTracker, le)
	return c
}

// GetConfig returns the config.
func (c *Controller) GetConfig() *Config {
	return c.c
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"bldr manifest builder: "+c.c.GetBuilderConfig().GetManifestMeta().GetManifestId(),
	)
}

// GetResultPromise returns the result promise.
func (c *Controller) GetResultPromise() *promise.PromiseContainer[*bldr_manifest_builder.BuilderResult] {
	return c.resultPromise
}

// SetManifestBuilderLifecycleSink sets the lifecycle status sink.
func (c *Controller) SetManifestBuilderLifecycleSink(sink ManifestBuilderLifecycleSink) {
	c.mtx.Lock()
	c.lifecycleSink = sink
	status := c.lifecycleStatus
	c.mtx.Unlock()
	if sink != nil {
		sink.SetManifestBuilderLifecycleStatus(status)
	}
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	c.subManifestBuilderTrackers.SetContext(ctx, true)
	c.resultPromise.SetPromise(nil)
	c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
		State:   ManifestBuilderLifecycleStateQueued,
		Summary: "queued",
	})

	ctx, err := bldr_manifest_builder.WithManifestCommitTimestampFromEnvironment(ctx)
	if err != nil {
		c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
			State:   ManifestBuilderLifecycleStateError,
			Summary: "parse manifest timestamp",
			Error:   err.Error(),
		})
		c.resultPromise.SetResult(nil, err)
		return err
	}

	builderConfig := c.GetConfig().GetBuilderConfig()
	meta := builderConfig.GetManifestMeta()
	manifestID := meta.GetManifestId()
	le := c.le.WithField("manifest-id", manifestID)
	controllerConfig := c.GetConfig().GetControllerConfig()

	le.Debugf("starting manifest build controller: %s", manifestID)
	c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
		State:   ManifestBuilderLifecycleStateRunning,
		Summary: "starting builder controller",
	})
	conf, err := controllerConfig.Resolve(ctx, c.bus)
	if err != nil {
		c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
			State:   ManifestBuilderLifecycleStateError,
			Summary: "resolve builder controller config",
			Error:   err.Error(),
		})
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// cast to a manifest_builder config
	pconf, ok := conf.GetConfig().(bldr_manifest_builder.ControllerConfig)
	if !ok {
		err := errors.Errorf(
			"config must implement bldr_manifest_builder.ControllerConfig interface: %s",
			conf.GetConfig().GetConfigID(),
		)
		c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
			State:   ManifestBuilderLifecycleStateError,
			Summary: "resolve builder controller config",
			Error:   err.Error(),
		})
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// set build backoff config
	execBackoff := func() backoff.BackOff {
		ebo := backoff.NewExponentialBackOff()
		ebo.InitialInterval = time.Second
		ebo.Multiplier = 2
		ebo.MaxInterval = time.Second * 10
		return ebo
	}

	nctx, nctxCancel := context.WithCancel(ctx)
	defer nctxCancel()

	var wasDisposed atomic.Bool
	builderCtrlInter, _, ctrlRef, err := loader.WaitExecControllerRunning(
		nctx,
		c.bus,
		resolver.NewLoadControllerWithConfigAndOpts(pconf, directive.ValueOptions{}, execBackoff),
		func() {
			wasDisposed.Store(true)
			nctxCancel()
		},
	)
	if err != nil {
		c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
			State:   ManifestBuilderLifecycleStateError,
			Summary: "start builder controller",
			Error:   err.Error(),
		})
		c.resultPromise.SetResult(nil, err)
		return err
	}
	defer ctrlRef.Release()

	builderCtrl, ok := builderCtrlInter.(bldr_manifest_builder.Controller)
	if !ok {
		err := errors.Errorf("builder must implement bldr_manifest_builder.Controller: %#v", builderCtrlInter)
		c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
			State:   ManifestBuilderLifecycleStateError,
			Summary: "start builder controller",
			Error:   err.Error(),
		})
		c.resultPromise.SetResult(nil, err)
		return err
	}

	var startupValidated bool
	buildOwner := newManifestBuildOwner(c, builderConfig)

	// manifestDepSnapshot holds the last-seen refs for watched manifest deps.
	// Passed as an immutable snapshot to the watcher goroutine.
	var manifestDepSnapshot map[string]*bucket.ObjectRef

	watchManifestIDs := c.c.GetWatchManifestIds()

	for {
		if ctx.Err() != nil {
			return context.Canceled
		}

		resultPromise := buildOwner.nextResultPromise()

		var result *bldr_manifest_builder.BuilderResult
		var err error
		cacheHit := false
		var buildManifestDeps []*bldr_manifest_builder.InputManifest_ManifestDep
		var buildManifestDepRefs map[string]*bucket.ObjectRef

		if !startupValidated {
			startupValidated = true
			startupValidationResult, startupErr := c.validateStartupBuilderResult(ctx, le, builderCtrl)
			if startupErr != nil {
				c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
					State:   ManifestBuilderLifecycleStateError,
					Summary: "validate startup cache",
					Error:   startupErr.Error(),
				})
				c.resultPromise.SetResult(nil, startupErr)
				return startupErr
			}
			if startupValidationResult.builderResult != nil {
				result = startupValidationResult.builderResult
				manifestDepSnapshot = startupValidationResult.manifestDepSnapshot
				cacheHit = true
				c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
					State:    ManifestBuilderLifecycleStateDone,
					CacheHit: true,
					Summary:  "startup cache hit",
				})
				for _, subManifestID := range startupValidationResult.subManifestIDs {
					le.WithField("manifest-id", subManifestID).
						WithField("startup-cache", true).
						Info("reused cached startup manifest build")
				}
				le.WithField("startup-cache", true).Info("reused cached startup manifest build")
			}
			if startupValidationResult.builderResult == nil {
				le.WithField("reason", startupValidationResult.reason).Info("startup manifest cache miss")
			}
		}

		attempt := buildOwner.beginAttempt(ctx)
		fullRebuild, hotRebuild := buildOwner.rebuildFlags(result)
		if result == nil {
			c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
				State:                   ManifestBuilderLifecycleStateRunning,
				FullRebuild:             fullRebuild,
				HotRebuild:              hotRebuild,
				DependencyRebuildReason: buildOwner.rebuildReasonSnapshot(),
				Summary:                 rebuildSummary(fullRebuild, hotRebuild),
			})
			args := buildOwner.buildArgs()

			if len(watchManifestIDs) != 0 {
				buildManifestDeps, buildManifestDepRefs = c.resolveManifestDeps(attempt.ctx, le, watchManifestIDs)
				le.WithField("watch-manifest-ids", watchManifestIDs).
					WithField("resolved-refs", len(buildManifestDepRefs)).
					Debug("resolved manifest dep refs for watching")
			}
			// The restart callback binds every nested manifest tracker to this attempt.
			builderHost := newBuildManifestHost(c, builderConfig, attempt.restart)
			for _, prevSubManifestTracker := range c.subManifestBuilderTrackers.GetKeysWithData() {
				tkr := prevSubManifestTracker.Data
				tkr.build.prepareParentAttempt(attempt.restart)
			}

			pluginBuildPermit, acquireErr := c.pluginBuildLimiter.Acquire(
				attempt.ctx,
				pconf.GetConfigID(),
			)
			if acquireErr != nil {
				attempt.release()
				return acquireErr
			}

			// Call the builder controller BuildManifest function.
			result, err = builderCtrl.BuildManifest(attempt.ctx, args, builderHost)
			pluginBuildPermit.Release()
			if ctx.Err() != nil {
				attempt.release()
				return context.Canceled
			}
			if attempt.ctx.Err() != nil {
				if attempt.wasRestarted() {
					attempt.release()
					continue
				}
			}
		}

		if err == nil && result != nil && len(watchManifestIDs) != 0 {
			// A one-shot parent can start before its provider and finish after
			// it. Persist the dependency state at parent completion, not the
			// incomplete pre-build snapshot.
			buildManifestDeps, buildManifestDepRefs = c.resolveManifestDeps(
				attempt.ctx,
				le,
				watchManifestIDs,
			)
		}

		// Delete sub-manifests that were not observed this run and persist the
		// observed child provenance with a newly built parent result.
		var subManifestCount int
		for _, prevSubManifestTracker := range c.subManifestBuilderTrackers.GetKeysWithData() {
			tkr := prevSubManifestTracker.Data
			subManifestResult, subManifestErr, observed := tkr.build.observedResult()
			if !observed {
				c.subManifestBuilderTrackers.RemoveKey(prevSubManifestTracker.Key)
				continue
			}
			subManifestCount++
			if err == nil && subManifestErr != nil {
				err = errors.Wrapf(subManifestErr, "persist sub-manifest %q", prevSubManifestTracker.Key)
			}
			if err == nil && result != nil && !cacheHit && subManifestResult != nil {
				err = addSubManifestResultForStartupReuse(
					result,
					prevSubManifestTracker.Key,
					subManifestResult,
				)
			}
		}
		if err == nil && !cacheHit {
			err = enrichBuilderResultForStartupReuse(builderConfig, c.c.GetControllerConfig(), result)
		}

		// Set the result promise
		// Only watch manifest deps if the build produced a result.
		// Compilers that skip a platform return nil result with nil error.
		hasManifestDeps := len(watchManifestIDs) > 0 && result != nil
		// Populate manifest_deps with current refs for watched manifests.
		if err == nil && hasManifestDeps && result.GetInputManifest() != nil && buildManifestDeps != nil {
			result.GetInputManifest().ManifestDeps = buildManifestDeps
			manifestDepSnapshot = buildManifestDepRefs
		}
		if attempt.wasRestarted() {
			attempt.release()
			continue
		}
		if err := buildOwner.publishResult(attempt.ctx, le, resultPromise, result, err, cacheHit, fullRebuild, hotRebuild); err != nil {
			if attempt.wasRestarted() || attempt.ctx.Err() != nil {
				attempt.release()
				continue
			}
			attempt.release()
			return err
		}

		// publishResult preserves prevResult after a successful build.
		inputFiles := buildOwner.prevResult.GetInputManifest().GetFiles()
		if err == nil {
			le.Debugf("input manifest returned with %d files", len(inputFiles))
		}
		if err != nil {
			le.WithError(err).Warn("build failed")
		}
		if !c.c.GetWatch() || (len(inputFiles) == 0 && subManifestCount == 0 && !hasManifestDeps) {
			attempt.release()
			return buildOwner.prevErr
		}

		// ignoreWatchPrefixes are prefixes to ignore from watching
		ignoreWatchPrefixes := []string{"vendor", "node_modules", ".bldr", "(disabled)"}

		// build file watchlist
		watchedFiles := make(map[string]*bldr_manifest_builder.InputManifest_File)
	FilesLoop:
		for _, inputFile := range inputFiles {
			if inputFile.GetStartupOnly() {
				continue
			}
			filePath := inputFile.GetPath()
			for _, prefix := range ignoreWatchPrefixes {
				if strings.HasPrefix(filePath, prefix) {
					continue FilesLoop
				}
			}
			if _, ok := watchedFiles[filePath]; !ok {
				watchedFiles[filePath] = inputFile
			}
		}

		if len(watchedFiles) == 0 {
			le.Debug("builder provided no files to watch")
			c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
				State:                   ManifestBuilderLifecycleStateDone,
				CacheHit:                cacheHit,
				FullRebuild:             fullRebuild,
				HotRebuild:              hotRebuild,
				WatchedFileCount:        0,
				DependencyRebuildReason: buildOwner.rebuildReasonSnapshot(),
				Summary:                 "watching for rebuild triggers",
			})

			if subManifestCount == 0 && !hasManifestDeps {
				// nothing to wait for, return.
				attempt.release()
				return nil
			}

			// Start manifest dep watcher if we have deps.
			if hasManifestDeps {
				go c.watchManifestDeps(attempt.ctx, le, watchManifestIDs, manifestDepSnapshot, attempt.restart)
			}

			// wait for sub-manifests/manifest-deps to change or ctx to cancel
			select {
			case <-attempt.ctx.Done():
				attempt.release()
				continue
			case <-ctx.Done():
				attempt.release()
				return context.Canceled
			}
		}

		// compare list of files with previous list of file
		watchedSourcePaths := make(map[string]*bldr_manifest_builder.InputManifest_File, len(watchedFiles))
		watchedSourceDirs := make(map[string]struct{}, len(watchedFiles))
		for filePath, v := range watchedFiles {
			sourcePath := filepath.Join(builderConfig.GetSourcePath(), filePath)
			watchedSourcePaths[sourcePath] = v
			sourceDir := filepath.Dir(sourcePath)
			if _, ok := watchedSourceDirs[sourceDir]; !ok {
				watchedSourceDirs[sourceDir] = struct{}{}
			}
		}

		// It's best to watch the entire directory tree and filter the events.
		//
		// This is both more efficient on the kernel side and avoids nasty quriks
		// with git and other editors deleting and re-creating files.
		//
		// See fsnotify comments:
		//   Watching individual files (rather than directories) is generally not
		//   recommended as many programs (especially editors) update files atomically: it
		//   will write to a temporary file which is then moved to to destination,
		//   overwriting the original (or some variant thereof). The watcher on the
		//   original file is now lost, as that no longer exists.
		//
		// It's necessary to create one watcher per directory:
		//   https://github.com/fsnotify/fsnotify/issues/18
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			attempt.release()
			return err
		}

		for watchedDirPath := range watchedSourceDirs {
			err = watcher.Add(watchedDirPath)
			if err != nil {
				attempt.release()
				_ = watcher.Close()
				return err
			}
		}

		// Start manifest dep watcher concurrently with file watcher.
		if hasManifestDeps {
			go c.watchManifestDeps(attempt.ctx, le, watchManifestIDs, manifestDepSnapshot, attempt.restart)
		}

		le.Debugf("watching for changes in %d files and %d directories and %d sub-manifests and %d manifest deps", len(watchedFiles), len(watchedSourceDirs), subManifestCount, len(watchManifestIDs))
		c.setLifecycleStatus(ManifestBuilderLifecycleStatus{
			State:                   ManifestBuilderLifecycleStateDone,
			CacheHit:                cacheHit,
			FullRebuild:             fullRebuild,
			HotRebuild:              hotRebuild,
			WatchedFileCount:        len(watchedFiles),
			DependencyRebuildReason: buildOwner.rebuildReasonSnapshot(),
			Summary:                 "watching for changes",
		})
		happened, err := debounce_fswatcher.DebounceFSWatcherEvents(
			attempt.ctx,
			watcher,
			time.Millisecond*100,
			func(event fsnotify.Event) (match bool, err error) {
				// filter for watchedSourcePaths
				if _, ok := watchedSourcePaths[event.Name]; !ok {
					return false, nil
				}
				return true, nil
			},
		)
		_ = watcher.Close()

		if ctx.Err() != nil {
			attempt.release()
			return context.Canceled
		}
		if attempt.ctx.Err() != nil {
			attempt.release()
			le.Info("re-building after sub-manifest or manifest dep changed")
			continue
		}
		if err != nil {
			attempt.release()
			return err
		}

		// build list of changed files
		// DebounceFSWatcherEvents watches for Create, Rename, Write, Remove
		// we know there is at least one event in happened
		var changedFiles []*bldr_manifest_builder.InputManifest_File
		seenChangedFiles := make(map[*bldr_manifest_builder.InputManifest_File]struct{}, len(happened))
		for _, event := range happened {
			inputFile := watchedSourcePaths[event.Name]
			if _, ok := seenChangedFiles[inputFile]; !ok && inputFile != nil {
				seenChangedFiles[inputFile] = struct{}{}
				changedFiles = append(changedFiles, inputFile)
			}
		}

		le.Infof("re-building after %d filesystem events with %d changed files", len(happened), len(changedFiles))
		buildOwner.setChangedFiles(changedFiles)
		attempt.release()
	}
}

// storeManifestBuildResult stores world-backed build provenance for startup reuse.
func (c *Controller) storeManifestBuildResult(
	ctx context.Context,
	le *logrus.Entry,
	result *bldr_manifest_builder.BuilderResult,
) error {
	builderConfig := c.c.GetBuilderConfig()
	if result.GetManifest().GetMeta().GetRev() != builderConfig.GetManifestMeta().GetRev() {
		le.WithField("manifest-rev", result.GetManifest().GetMeta().GetRev()).
			Debug("skipping world-backed manifest build result for reused manifest")
		return nil
	}
	busEngine := world.NewBusEngine(ctx, c.bus, builderConfig.GetEngineId())
	tx, err := busEngine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	ref, err := resultworld.SetManifestBuildResult(ctx, tx, builderConfig.GetObjectKey(), result)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	le.WithField("object-key", resultworld.ManifestBuildResultKey(builderConfig.GetObjectKey())).
		WithField("ref", ref).
		Debug("stored manifest build result in world")
	return nil
}

// collectManifestRefs collects current refs for the given manifest IDs from the world.
func (c *Controller) collectManifestRefs(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	manifestIDs []string,
) map[string]*bucket.ObjectRef {
	builderConfig := c.c.GetBuilderConfig()
	linkObjKeys := builderConfig.GetLinkObjectKeys()
	var platformIDs []string
	if platformID := builderConfig.GetManifestMeta().GetPlatformId(); platformID != "" {
		platformIDs = []string{platformID}
	}
	manifests, manifestErrs, err := bldr_manifest_world.CollectManifests(
		ctx,
		ws,
		platformIDs,
		linkObjKeys...,
	)
	if err != nil {
		le.WithError(err).Warn("failed to collect manifest refs")
		return nil
	}
	for _, manifestErr := range manifestErrs {
		le.WithError(manifestErr).Warn("skipping invalid manifest")
	}

	refs := make(map[string]*bucket.ObjectRef, len(manifestIDs))
	for _, id := range manifestIDs {
		if collected := manifests[id]; len(collected) > 0 {
			refs[id] = collected[0].ManifestRef
		}
	}
	return refs
}

// resolveManifestDeps resolves current refs for watched manifest IDs
// and returns InputManifest_ManifestDep entries.
func (c *Controller) resolveManifestDeps(
	ctx context.Context,
	le *logrus.Entry,
	watchManifestIDs []string,
) ([]*bldr_manifest_builder.InputManifest_ManifestDep, map[string]*bucket.ObjectRef) {
	engineID := c.c.GetBuilderConfig().GetEngineId()
	busEngine := world.NewBusEngine(ctx, c.bus, engineID)
	ws := world.NewEngineWorldState(busEngine, false)
	refs := c.collectManifestRefs(ctx, le, ws, watchManifestIDs)

	deps := make([]*bldr_manifest_builder.InputManifest_ManifestDep, 0, len(watchManifestIDs))
	for _, id := range watchManifestIDs {
		deps = append(deps, &bldr_manifest_builder.InputManifest_ManifestDep{
			ManifestId:  id,
			ManifestRef: refs[id],
		})
	}
	return deps, refs
}

// watchManifestDeps watches the world for changes to manifest dependencies.
// Calls restartFn when a watched manifest's ref changes from the snapshot.
// The snapshot is an immutable copy; this function does not write to shared state.
func (c *Controller) watchManifestDeps(
	ctx context.Context,
	le *logrus.Entry,
	watchManifestIDs []string,
	snapshot map[string]*bucket.ObjectRef,
	restartFn func(string),
) {
	le.WithField("watch-manifest-ids", watchManifestIDs).
		WithField("snapshot-size", len(snapshot)).
		Debug("starting manifest dep watcher")
	engineID := c.c.GetBuilderConfig().GetEngineId()
	objLoop := world_control.NewWatchLoop(
		le.WithField("watch", "manifest-deps"),
		"",
		func(
			ctx context.Context,
			le *logrus.Entry,
			ws world.WorldState,
			_ world.ObjectState,
			_ *bucket.ObjectRef,
			_ uint64,
		) (bool, error) {
			refs := c.collectManifestRefs(ctx, le, ws, watchManifestIDs)
			for _, id := range watchManifestIDs {
				prev := snapshot[id]
				curr := refs[id]
				if curr == nil {
					continue
				}
				// Trigger rebuild if the ref changed or if the manifest
				// appeared for the first time since the snapshot was taken.
				if prev == nil || !curr.EqualVT(prev) {
					le.WithField("changed-manifest", id).
						Info("manifest dependency changed, triggering rebuild")
					restartFn("manifest dependency changed: " + id)
					return false, nil
				}
			}
			return true, nil
		},
	)

	if err := world_control.ExecuteBusWatchLoop(ctx, c.bus, engineID, false, objLoop); err != nil && err != context.Canceled && ctx.Err() == nil {
		le.WithError(err).Warn("manifest dep watcher exited with error")
	}
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)

func (c *Controller) setLifecycleStatus(status ManifestBuilderLifecycleStatus) {
	c.mtx.Lock()
	c.lifecycleStatus = status
	sink := c.lifecycleSink
	c.mtx.Unlock()
	if sink != nil {
		sink.SetManifestBuilderLifecycleStatus(status)
	}
}

func rebuildSummary(fullRebuild, hotRebuild bool) string {
	if hotRebuild {
		return "hot rebuild"
	}
	if fullRebuild {
		return "full rebuild"
	}
	return "build"
}

// changedFilesSummary summarizes a changed-file count for lifecycle status.
func changedFilesSummary(count int) string {
	switch count {
	case 0:
		return "filesystem change"
	case 1:
		return "filesystem change: 1 changed file"
	default:
		return "filesystem change: multiple changed files"
	}
}
