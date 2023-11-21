package bldr_manifest_builder_controller

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	debounce_fswatcher "github.com/aperturerobotics/util/debounce-fswatcher"
	"github.com/aperturerobotics/util/promise"
	"github.com/blang/semver"
	"github.com/cenkalti/backoff"
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
	resultPromise *promise.PromiseContainer[*manifest_builder.BuilderResult]
}

// NewController constructs a new controller.
func NewController(le *logrus.Entry, bus bus.Bus, cc *Config) *Controller {
	return &Controller{
		le:            le,
		bus:           bus,
		c:             cc,
		resultPromise: promise.NewPromiseContainer[*manifest_builder.BuilderResult](),
	}
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
func (c *Controller) GetResultPromise() promise.PromiseLike[*manifest_builder.BuilderResult] {
	return c.resultPromise
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
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
	pconf, ok := conf.GetConfig().(manifest_builder.ControllerConfig)
	if !ok {
		err := errors.Errorf(
			"config must implement manifest_builder.ControllerConfig interface: %s",
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

	builderCtrl, ok := builderCtrlInter.(manifest_builder.Controller)
	if !ok {
		err := errors.Errorf("builder must implement manifest_builder.Controller: %#v", builderCtrlInter)
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// Watcher
	var watcher *fsnotify.Watcher
	var watchedFiles map[string]*manifest_builder.InputManifest_File
	if c.c.GetWatch() {
		watcher, err = fsnotify.NewWatcher()
		if err != nil {
			return err
		}
		defer watcher.Close()
		watchedFiles = make(map[string]*manifest_builder.InputManifest_File)
	}

	var prevResult *manifest_builder.BuilderResult
	var prevErr error
	var changedFiles []*manifest_builder.InputManifest_File
	for {
		resultPromise := promise.NewPromise[*manifest_builder.BuilderResult]()
		c.resultPromise.SetPromise(resultPromise)

		args := &manifest_builder.BuildManifestArgs{
			BuilderConfig: builderConfig,

			PrevBuilderResult: prevResult,
			ChangedFiles:      changedFiles,
		}
		changedFiles = nil
		result, err := builderCtrl.BuildManifest(ctx, args)
		resultPromise.SetResult(result, err)
		if err == nil {
			prevResult = result
		}
		prevErr = err

		inputFiles := prevResult.GetInputManifest().GetFiles()
		if err == nil {
			le.Debugf("input manifest returned with %d files", len(inputFiles))
		} else {
			le.WithError(err).Warn("build failed")
		}

		if !c.c.GetWatch() || len(inputFiles) == 0 {
			return prevErr
		}

		// ignoreWatchPrefixes are prefixes to ignore from watching
		ignoreWatchPrefixes := []string{"vendor", "node_modules", ".bldr", "(disabled)"}

		// build file watchlist
		nextWatchedFiles := make(map[string]*manifest_builder.InputManifest_File)
	FilesLoop:
		for _, inputFile := range inputFiles {
			filePath := inputFile.GetPath()
			for _, prefix := range ignoreWatchPrefixes {
				if strings.HasPrefix(filePath, prefix) {
					continue FilesLoop
				}
			}
			if _, ok := nextWatchedFiles[filePath]; !ok {
				nextWatchedFiles[filePath] = inputFile
			}
		}

		if len(nextWatchedFiles) == 0 {
			le.Debug("builder provided no files to watch, returning")
			return nil
		}

		// compare list of files with previous list of file
		watchedSourcePaths := make(map[string]*manifest_builder.InputManifest_File, len(nextWatchedFiles)+len(watchedFiles))
		for filePath := range watchedFiles {
			sourcePath := filepath.Join(builderConfig.GetSourcePath(), filePath)
			if v, ok := nextWatchedFiles[filePath]; ok {
				// file was already watched: update the file pointer
				watchedFiles[filePath] = v
				watchedSourcePaths[sourcePath] = v
				delete(nextWatchedFiles, filePath)
				continue
			}

			le.Debugf("removing watcher for file: %s", filePath)
			delete(watchedFiles, filePath)
			delete(watchedSourcePaths, sourcePath)
			if err := watcher.Remove(filePath); err != nil {
				return err
			}
		}

		for filePath, v := range nextWatchedFiles {
			sourcePath := filepath.Join(builderConfig.GetSourcePath(), filePath)
			watchedFiles[filePath] = v
			watchedSourcePaths[sourcePath] = v
			if err := watcher.Add(sourcePath); err != nil {
				le.WithError(err).Warnf("unable to add watcher for file: %s", filePath)
			} else {
				le.Debugf("adding watcher for file: %s", filePath)
			}
		}

		// wait for a file change
		le.Debugf("watching for changes in %d files", len(watchedFiles))
		happened, err := debounce_fswatcher.DebounceFSWatcherEvents(
			ctx,
			watcher,
			time.Millisecond*250,
		)
		if err != nil {
			return err
		}

		// build list of changed files
		// DebounceFSWatcherEvents watches for Create, Rename, Write, Remove
		seenChangedFiles := make(map[*manifest_builder.InputManifest_File]struct{}, len(happened))
		for _, event := range happened {
			inputFile := watchedSourcePaths[event.Name]

			// Shouldn't happen: but log a warning if so.
			if inputFile == nil {
				le.
					WithField("watched-path", event.Name).
					Warn("filesystem event does not correspond to any watched files")
				continue
			}

			if _, ok := seenChangedFiles[inputFile]; !ok {
				seenChangedFiles[inputFile] = struct{}{}
				changedFiles = append(changedFiles, inputFile)
			}
		}

		le.Infof("re-building after %d filesystem events with %d changed files", len(happened), len(changedFiles))
	}
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
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
