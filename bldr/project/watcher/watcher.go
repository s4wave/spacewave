//go:build !js

package bldr_project_watcher

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/fsnotify"
	"github.com/aperturerobotics/util/ccontainer"
	debounce_fswatcher "github.com/aperturerobotics/util/debounce-fswatcher"
	"github.com/pkg/errors"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "bldr project config file watcher"

// ControllerID is the ID of the controller.
const ControllerID = ConfigID

// Controller is the bldr Project Watcher controller.
type Controller struct {
	*bus.BusController[*Config]
	// startCtr holds the startup outcome, nil while the first load runs.
	startCtr *ccontainer.CContainer[*startResult]
	// starLoadedFiles tracks files loaded during the last starlark evaluation.
	starLoadedFiles []string
}

// Factory is the factory for the controller.
type Factory = bus.BusFactory[*Config, *Controller]

// NewFactory constructs a new controller factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config { return &Config{} },
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{
				BusController: base,
				startCtr:      ccontainer.NewCContainer[*startResult](nil),
			}, nil
		},
	)
}

// startResult is the outcome of starting the project controller: the running
// controller, or the error of a start that nothing will retry.
type startResult struct {
	ctrl *bldr_project_controller.Controller
	err  error
}

// WaitProjectController waits for the running project controller. Without a
// watch, a failed start is final and returns its error. A watching controller
// retries the start until the config loads.
func (c *Controller) WaitProjectController(ctx context.Context) (*bldr_project_controller.Controller, error) {
	res, err := c.startCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return res.ctrl, res.err
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(rctx context.Context) error {
	// Run the controller under a context that cancels with the lifecycle.
	ctx, subCtxCancel := context.WithCancel(rctx)
	defer subCtxCancel()

	// Start the project controller. Without a watch the config cannot change,
	// so a failed start is final rather than retried.
	configPath := c.GetConfig().GetConfigPath()
	watch := !c.GetConfig().GetDisableWatch() && configPath != ""
	projCtrl, ctrlRef, err := c.start(ctx, subCtxCancel)
	if err != nil {
		if watch || ctx.Err() != nil {
			return err
		}
		c.startCtr.SetValue(&startResult{err: err})
		return nil
	}
	defer ctrlRef.Release()
	c.startCtr.SetValue(&startResult{ctrl: projCtrl})

	// Block until canceled when not watching.
	if !watch {
		<-ctx.Done()
		return context.Canceled
	}

	// Watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Resolve the bldr.star path beside the config path.
	starPath := ResolveStarlarkPath(configPath)

	// Watch the config files and restart the project controller on changes.
	for {
		// Missing paths are expected here: bldr.star and loaded files may not
		// exist yet. Other Add failures (e.g. too many watches) are dropped
		// too; the debounce loop still sees changes on successfully added
		// paths.
		_ = watcher.Add(configPath)
		_ = watcher.Add(starPath)
		for _, f := range c.starLoadedFiles {
			_ = watcher.Add(f)
		}
		// wait for a file change
		happened, err := debounce_fswatcher.DebounceFSWatcherEvents(
			ctx,
			watcher,
			time.Millisecond*500,
			nil,
		)
		if err != nil {
			return err
		}
		var restart bool
		for _, event := range happened {
			if event.Op != fsnotify.Chmod {
				restart = true
				break
			}
		}
		if !restart {
			continue
		}

		// load the new config
		updConf, err := c.loadProjectControllerConfig(ctx)
		if err != nil {
			return err
		}

		// apply to the controller
		if err := projCtrl.UpdateProjectConfig(updConf.GetProjectConfig()); err != nil {
			return err
		}
	}
}

// start loads the config and waits for the project controller it configures
// to run. disposed is called when the project controller stops running.
func (c *Controller) start(ctx context.Context, disposed func()) (*bldr_project_controller.Controller, directive.Reference, error) {
	// Load the initial config.
	projCtrlConf, err := c.loadProjectControllerConfig(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Start the controller.
	ctrl, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		c.GetBus(),
		resolver.NewLoadControllerWithConfig(projCtrlConf),
		disposed,
	)
	if err != nil {
		return nil, nil, err
	}
	projCtrl, ok := ctrl.(*bldr_project_controller.Controller)
	if !ok {
		ctrlRef.Release()
		return nil, nil, errors.New("project controller returned with unknown type")
	}
	return projCtrl, ctrlRef, nil
}

// loadProjectControllerConfig loads a merged copy of the project controller config.
func (c *Controller) loadProjectControllerConfig(ctx context.Context) (*bldr_project_controller.Config, error) {
	// Start from a clone of the configured project controller config.
	ctrlConfig := c.GetConfig().GetProjectControllerConfig().CloneVT()
	if ctrlConfig == nil {
		ctrlConfig = &bldr_project_controller.Config{}
	}
	if ctrlConfig.ProjectConfig == nil {
		ctrlConfig.ProjectConfig = &bldr_project.ProjectConfig{}
	}

	// Merge the bldr.yaml and bldr.star configs when a config path is set.
	configPath := c.GetConfig().GetConfigPath()
	if configPath != "" {
		starPath := ResolveStarlarkPath(configPath)
		sourcePath := filepath.Dir(configPath)

		// Load bldr.yaml if it exists.
		yamlData, yamlErr := os.ReadFile(configPath)
		_, starErr := os.Stat(starPath)
		if yamlErr != nil && starErr != nil {
			// Neither file exists.
			return nil, yamlErr
		}

		if yamlErr == nil {
			projConfig := &bldr_project.ProjectConfig{}
			if err := bldr_project.UnmarshalProjectConfig(yamlData, projConfig); err != nil {
				return nil, errors.Wrap(err, "unmarshal project config")
			}
			if err := projConfig.Validate(); err != nil {
				return nil, err
			}

			// resolve extends: merge extended project configs first (in order)
			for _, modulePath := range projConfig.GetExtends() {
				extConfig, _, err := bldr_project.LoadExtendedProjectConfig(sourcePath, modulePath)
				if err != nil {
					return nil, errors.Wrapf(err, "extends %s", modulePath)
				}
				if err := bldr_project.MergeProjectConfigs(ctrlConfig.ProjectConfig, extConfig); err != nil {
					return nil, errors.Wrapf(err, "merge extends %s", modulePath)
				}
			}

			// merge local yaml config on top of extended configs
			if err := bldr_project.MergeProjectConfigs(ctrlConfig.ProjectConfig, projConfig); err != nil {
				return nil, err
			}
		}

		// Evaluate bldr.star if it exists.
		if starErr == nil {
			result, err := bldr_project_starlark.Evaluate(starPath, bldr_project_starlark.WithVars(c.GetConfig().GetVars()))
			if err != nil {
				return nil, errors.Wrap(err, "evaluate bldr.star")
			}

			// If bldr.star specifies extends, resolve them.
			for _, modulePath := range result.Config.GetExtends() {
				extConfig, _, err := bldr_project.LoadExtendedProjectConfig(sourcePath, modulePath)
				if err != nil {
					return nil, errors.Wrapf(err, "extends %s", modulePath)
				}
				if err := bldr_project.MergeProjectConfigs(ctrlConfig.ProjectConfig, extConfig); err != nil {
					return nil, errors.Wrapf(err, "merge extends %s", modulePath)
				}
			}
			// Clear extends before merge so they are not re-processed.
			result.Config.Extends = nil

			if err := bldr_project.MergeProjectConfigs(ctrlConfig.ProjectConfig, result.Config); err != nil {
				return nil, errors.Wrap(err, "merge bldr.star config")
			}
			c.starLoadedFiles = result.LoadedFiles
		} else {
			c.starLoadedFiles = nil
		}
	}

	// Overlay the controller's bound remotes onto the project config.
	if len(c.GetConfig().GetBoundRemotes()) != 0 {
		if ctrlConfig.ProjectConfig.Remotes == nil {
			ctrlConfig.ProjectConfig.Remotes = make(map[string]*bldr_project.RemoteConfig)
		}
		for id, remote := range c.GetConfig().GetBoundRemotes() {
			ctrlConfig.ProjectConfig.Remotes[id] = remote.CloneVT()
		}
	}
	return ctrlConfig, nil
}

// ResolveStarlarkPath returns the bldr.star path beside a config path.
// For example, "path/to/bldr.yaml" resolves to "path/to/bldr.star".
func ResolveStarlarkPath(configPath string) string {
	dir := filepath.Dir(configPath)
	return filepath.Join(dir, "bldr.star")
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
