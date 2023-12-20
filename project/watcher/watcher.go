package bldr_project_watcher

import (
	"context"
	"os"
	"time"

	bldr_project "github.com/aperturerobotics/bldr/project"
	bldr_project_controller "github.com/aperturerobotics/bldr/project/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/ccontainer"
	debounce_fswatcher "github.com/aperturerobotics/util/debounce-fswatcher"
	"github.com/blang/semver"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "bldr project config file watcher"

// ControllerID is the ID of the controller.
const ControllerID = "bldr/project/watcher"

// Controller is the bldr Project Watcher controller.
type Controller struct {
	*bus.BusController[*Config]
	// projCtrlCtr is the project controller container
	projCtrlCtr *ccontainer.CContainer[*bldr_project_controller.Controller]
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
				projCtrlCtr:   ccontainer.NewCContainer[*bldr_project_controller.Controller](nil),
			}, nil
		},
	)
}

// GetProjectController returns the project controller watchable.
func (c *Controller) GetProjectController() ccontainer.Watchable[*bldr_project_controller.Controller] {
	return c.projCtrlCtr
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(rctx context.Context) error {
	ctx, subCtxCancel := context.WithCancel(rctx)
	defer subCtxCancel()

	// load the initial config
	projCtrlConf, err := c.loadProjectControllerConfig(ctx)
	if err != nil {
		return err
	}

	// start the controller
	ctrl, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		c.GetBus(),
		resolver.NewLoadControllerWithConfig(projCtrlConf),
		subCtxCancel,
	)
	if err != nil {
		return err
	}
	defer ctrlRef.Release()

	projCtrl, ok := ctrl.(*bldr_project_controller.Controller)
	if !ok {
		return errors.New("project controller returned with unknown type")
	}
	c.projCtrlCtr.SetValue(projCtrl)

	configPath := c.GetConfig().GetConfigPath()
	if c.GetConfig().GetDisableWatch() || configPath == "" {
		<-ctx.Done()
		return context.Canceled
	}

	// Watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	for {
		// add the config file (or re-add if watcher was removed)
		if err := watcher.Add(configPath); err != nil {
			return err
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

// loadProjectControllerConfig loads a merged copy of the project controller config.
func (c *Controller) loadProjectControllerConfig(ctx context.Context) (*bldr_project_controller.Config, error) {
	ctrlConfig := c.GetConfig().GetProjectControllerConfig().CloneVT()
	if ctrlConfig == nil {
		ctrlConfig = &bldr_project_controller.Config{}
	}
	if ctrlConfig.ProjectConfig == nil {
		ctrlConfig.ProjectConfig = &bldr_project.ProjectConfig{}
	}

	configPath := c.GetConfig().GetConfigPath()
	if configPath != "" {
		projConfig := &bldr_project.ProjectConfig{}
		projConfYaml, err := os.ReadFile(configPath)
		if err != nil {
			return nil, err
		}
		if err := bldr_project.UnmarshalProjectConfig(projConfYaml, projConfig); err != nil {
			return nil, errors.Wrap(err, "unmarshal project config")
		}
		if err := projConfig.Validate(); err != nil {
			return nil, err
		}

		// merge
		if err := bldr_project.MergeProjectConfigs(ctrlConfig.ProjectConfig, projConfig); err != nil {
			return nil, err
		}
	}

	return ctrlConfig, nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
