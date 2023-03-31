package bldr_project_watcher

import (
	"bytes"
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
	"github.com/aperturerobotics/util/routine"
	"github.com/blang/semver"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/zeebo/blake3"
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
	// routine manages the project controller routine
	routine *routine.RoutineContainer
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
				routine: routine.NewRoutineContainer(
					routine.WithExitLogger(base.GetLogger().WithField("routine", "project-watcher")),
				),
				projCtrlCtr: ccontainer.NewCContainer[*bldr_project_controller.Controller](nil),
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
func (c *Controller) Execute(ctx context.Context) error {
	// determine the initial file hash
	fileHash, err := c.hashProjectFile()
	if err != nil {
		return err
	}

	// set the context on the routine container
	c.routine.SetContext(ctx, true)

	// set the routine
	c.routine.SetRoutine(c.executeProjectController)
	defer c.routine.SetRoutine(nil)

	configPath := c.GetConfig().GetConfigPath()
	if c.GetConfig().GetDisableWatch() || configPath == "" {
		return c.routine.WaitExited(ctx, nil)
	}

	// Watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// add the config file
	if err := watcher.Add(configPath); err != nil {
		return err
	}

	for {
		// wait for a file change
		happened, err := debounce_fswatcher.DebounceFSWatcherEvents(
			ctx,
			watcher,
			time.Millisecond*500,
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

		// check if the file actually changed
		nextHash, err := c.hashProjectFile()
		if err != nil {
			return err
		}
		if bytes.Equal(nextHash, fileHash) {
			// ignore, no changes
			continue
		}

		fileHash = nextHash
		c.routine.RestartRoutine()
	}
}

// hashProjectFile hashes the contents of the project file.
func (c *Controller) hashProjectFile() ([]byte, error) {
	configPath := c.GetConfig().GetConfigPath()
	if configPath == "" {
		return nil, nil
	}

	dat, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	sum := blake3.Sum256(dat)
	return sum[:], nil
}

// executeProjectController executes the ProjectController once.
func (c *Controller) executeProjectController(ctx context.Context) error {
	projConfig := &bldr_project.ProjectConfig{}
	configPath := c.GetConfig().GetConfigPath()
	if configPath != "" {
		projConfYaml, err := os.ReadFile(configPath)
		if err != nil {
			return err
		}
		if err := bldr_project.UnmarshalProjectConfig(projConfYaml, projConfig); err != nil {
			return errors.Wrap(err, "unmarshal project config")
		}
		if err := projConfig.Validate(); err != nil {
			return err
		}
	}

	ctrlConfig := c.GetConfig().GetProjectControllerConfig().CloneVT()
	if ctrlConfig == nil {
		ctrlConfig = &bldr_project_controller.Config{}
	}
	if ctrlConfig.ProjectConfig == nil {
		ctrlConfig.ProjectConfig = &bldr_project.ProjectConfig{}
	}

	// merge
	if err := bldr_project.MergeProjectConfigs(ctrlConfig.ProjectConfig, projConfig); err != nil {
		return err
	}

	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()
	ctrl, _, ctrlRef, err := loader.WaitExecControllerRunning(
		subCtx,
		c.GetBus(),
		resolver.NewLoadControllerWithConfig(ctrlConfig),
		subCtxCancel,
	)
	if err != nil {
		return err
	}
	projCtrl, ok := ctrl.(*bldr_project_controller.Controller)
	if !ok {
		return errors.New("project controller returned with unknown type")
	}
	c.projCtrlCtr.SetValue(projCtrl)
	<-subCtx.Done()
	c.projCtrlCtr.SwapValue(func(val *bldr_project_controller.Controller) *bldr_project_controller.Controller {
		if val == projCtrl {
			return nil
		}
		return val
	})
	ctrlRef.Release()
	// wait a moment before restarting
	<-time.After(time.Millisecond * 100)
	return context.Canceled
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
