//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	backoff "github.com/aperturerobotics/util/backoff/cbackoff"
	debounce_fswatcher "github.com/aperturerobotics/util/debounce-fswatcher"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/blang/semver/v4"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bldr/manifest/builder/controller"

// Controller is the builder controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// c is the controller config
	c *Config
	// resultPromise contains the result of the compilation.
	resultPromise *promise.PromiseContainer[*bldr_manifest_builder.BuilderResult]
	// subManifestBuilderTrackers track building sub-manifests
	subManifestBuilderTrackers *keyed.Keyed[string, *subManifestBuilderTracker]
}

// NewController constructs a new controller.
func NewController(le *logrus.Entry, bus bus.Bus, cc *Config) *Controller {
	c := &Controller{
		le:            le,
		bus:           bus,
		c:             cc,
		resultPromise: promise.NewPromiseContainer[*bldr_manifest_builder.BuilderResult](),
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

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	c.subManifestBuilderTrackers.SetContext(ctx, true)
	c.resultPromise.SetPromise(nil)

	builderConfig := c.GetConfig().GetBuilderConfig()
	meta := builderConfig.GetManifestMeta()
	manifestID := meta.GetManifestId()
	le := c.le.WithField("manifest-id", manifestID)
	controllerConfig := c.GetConfig().GetControllerConfig()

	le.Debugf("starting manifest build controller: %s", manifestID)
	conf, err := controllerConfig.Resolve(ctx, c.bus)
	if err != nil {
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
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// set build backoff config
	execBackoff := func() backoff.BackOff {
		ebo := backoff.NewExponentialBackOff()
		ebo.InitialInterval = time.Second
		ebo.Multiplier = 2
		ebo.MaxInterval = time.Second * 10
		// ebo.MaxElapsedTime = time.Minute
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
		c.resultPromise.SetResult(nil, err)
		return err
	}
	defer ctrlRef.Release()

	builderCtrl, ok := builderCtrlInter.(bldr_manifest_builder.Controller)
	if !ok {
		err := errors.Errorf("builder must implement bldr_manifest_builder.Controller: %#v", builderCtrlInter)
		c.resultPromise.SetResult(nil, err)
		return err
	}

	var prevResult *bldr_manifest_builder.BuilderResult
	var prevErr error
	var changedFiles []*bldr_manifest_builder.InputManifest_File

	// TODO: We do not increment the manifest revision when hot reloading.
	// TODO: Should that be done here?

	for {
		if ctx.Err() != nil {
			return context.Canceled
		}

		resultPromise := promise.NewPromise[*bldr_manifest_builder.BuilderResult]()
		c.resultPromise.SetPromise(resultPromise)

		args := &bldr_manifest_builder.BuildManifestArgs{
			BuilderConfig: builderConfig,

			PrevBuilderResult: prevResult,
			ChangedFiles:      changedFiles,
		}

		// buildCtx is for this build call
		buildCtx, buildCtxCancel := context.WithCancel(ctx)

		// restartFn forces restarting BuildManifest (once)
		var restarted atomic.Bool
		restartFn := func() {
			if !restarted.Swap(true) {
				buildCtxCancel()
			}
		}

		// construct the builder host which will set the restartFn when necessary
		builderHost := newBuildManifestHost(c, builderConfig, restartFn)

		// update restartFn on any existing manifest trackers
		for _, prevSubManifestTracker := range c.subManifestBuilderTrackers.GetKeysWithData() {
			tkr := prevSubManifestTracker.Data
			tkr.mtx.Lock()
			tkr.restartFn = restartFn
			tkr.resultPcObserved = false // flag that we shouldn't call restart() if the value changes (yet)
			tkr.mtx.Unlock()
		}

		// Call the builder controller BuildManifest function.
		changedFiles = nil
		result, err := builderCtrl.BuildManifest(buildCtx, args, builderHost)
		if ctx.Err() != nil {
			buildCtxCancel()
			return context.Canceled
		}
		if buildCtx.Err() != nil {
			if restarted.Load() {
				continue
			}
		}

		// Delete any sub-manifests that were not observed this run
		var subManifestCount int
		for _, prevSubManifestTracker := range c.subManifestBuilderTrackers.GetKeysWithData() {
			tkr := prevSubManifestTracker.Data
			tkr.mtx.Lock()
			resultPcObserved := tkr.resultPcObserved
			tkr.mtx.Unlock()
			if !resultPcObserved {
				c.subManifestBuilderTrackers.RemoveKey(prevSubManifestTracker.Key)
			} else {
				subManifestCount++
			}
		}

		// Set the result promise
		if err == nil {
			resultPromise.SetResult(result, nil)
			prevResult = result
		} else {
			resultPromise.SetResult(nil, err)
		}
		prevErr = err

		// NOTE: prevResult is the most recent result iif err == nil
		inputFiles := prevResult.GetInputManifest().GetFiles()
		if err == nil {
			le.Debugf("input manifest returned with %d files", len(inputFiles))
		} else {
			le.WithError(err).Warn("build failed")
		}

		if !c.c.GetWatch() || (len(inputFiles) == 0 && subManifestCount == 0) {
			buildCtxCancel()
			return prevErr
		}

		// ignoreWatchPrefixes are prefixes to ignore from watching
		ignoreWatchPrefixes := []string{"vendor", "node_modules", ".bldr", "(disabled)"}

		// build file watchlist
		watchedFiles := make(map[string]*bldr_manifest_builder.InputManifest_File)
	FilesLoop:
		for _, inputFile := range inputFiles {
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

			if subManifestCount == 0 {
				// nothing to wait for, return.
				buildCtxCancel()
				return nil
			}

			// wait for sub-manifests to change or ctx to cancel
			select {
			case <-buildCtx.Done():
				continue
			case <-ctx.Done():
				buildCtxCancel()
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
			buildCtxCancel()
			return err
		}

		for watchedDirPath := range watchedSourceDirs {
			err = watcher.Add(watchedDirPath)
			if err != nil {
				buildCtxCancel()
				_ = watcher.Close()
				return err
			}
		}

		le.Debugf("watching for changes in %d files and %d directories and %d sub-manifests", len(watchedFiles), len(watchedSourceDirs), subManifestCount)
		happened, err := debounce_fswatcher.DebounceFSWatcherEvents(
			buildCtx,
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
			buildCtxCancel()
			return context.Canceled
		}
		if buildCtx.Err() != nil {
			le.Info("re-building after sub-manifest changed")
			continue
		}
		if err != nil {
			buildCtxCancel()
			return err
		}

		// build list of changed files
		// DebounceFSWatcherEvents watches for Create, Rename, Write, Remove
		// we know there is at least one event in happened
		seenChangedFiles := make(map[*bldr_manifest_builder.InputManifest_File]struct{}, len(happened))
		for _, event := range happened {
			inputFile := watchedSourcePaths[event.Name]
			if _, ok := seenChangedFiles[inputFile]; !ok && inputFile != nil {
				seenChangedFiles[inputFile] = struct{}{}
				changedFiles = append(changedFiles, inputFile)
			}
		}

		le.Infof("re-building after %d filesystem events with %d changed files", len(happened), len(changedFiles))
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
var _ controller.Controller = ((*Controller)(nil))
