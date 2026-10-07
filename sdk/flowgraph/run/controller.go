// Package flowgraph_run drives the Steps of a Flowgraph run on a Forge Job.
package flowgraph_run

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "flowgraph/run"

// Controller drives one run object. It watches the run and the Tasks of its
// running activations, and advances the run in one World transaction per
// change: it routes each completed activation and starts each ready Step.
type Controller struct {
	// le is the root logger.
	le *logrus.Entry
	// conf is the config.
	conf *Config
	// engine is the World engine holding the run.
	engine *world.BusEngine
	// loop watches the run object.
	loop *world_control.WatchLoop
	// tasks watches the Task of each running activation.
	tasks *keyed.Keyed[string, *taskWatcher]
}

// NewController constructs a run controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) *Controller {
	c := &Controller{
		le:     le,
		conf:   conf,
		engine: world.NewBusEngine(nil, b, conf.GetEngineId()),
	}
	c.loop = world_control.NewWatchLoop(
		le.WithField("object-loop", "flowgraph-run"),
		conf.GetObjectKey(),
		c.processState,
	)
	c.tasks = keyed.NewKeyedWithLogger(
		c.newTaskWatcher,
		le,
		keyed.WithRetry[string, *taskWatcher](&backoff.Backoff{}),
	)
	return c
}

// StartControllerWithConfig starts a run controller and waits for it to run.
// Release the returned reference to stop the controller.
func StartControllerWithConfig(
	ctx context.Context,
	b bus.Bus,
	conf *Config,
) (*Controller, directive.Reference, error) {
	// Load the controller and wait for it to execute.
	ctrl, _, ref, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	// Return the loaded controller with its reference.
	c, ok := ctrl.(*Controller)
	if !ok {
		ref.Release()
		return nil, nil, block.ErrUnexpectedType
	}
	return c, ref, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "flowgraph run controller")
}

// Execute watches the run until the context ends. The Task watchers share
// the context and stop with it.
func (c *Controller) Execute(ctx context.Context) error {
	// Bind the Task watchers and the World engine to the controller context.
	c.tasks.SetContext(ctx, true)
	c.engine.SetContext(ctx)

	// Watch the run object until the context ends.
	err := c.loop.Execute(ctx, world.NewEngineWorldState(c.engine, true))
	if ctx.Err() != nil && errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return err
}

// processState advances the run and watches the Tasks it is waiting on.
func (c *Controller) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState,
	rootRef *bucket.ObjectRef,
	rev uint64,
) (bool, error) {
	// Wait for the run object to exist.
	if obj == nil {
		le.Debug("run object does not exist, waiting")
		c.tasks.SyncKeys(nil, true)
		return true, nil
	}

	// Advance the run atomically; a concurrent writer replays the attempt.
	var running []string
	err := world.ExecTransaction(ctx, c.engine, true, func(ctx context.Context, tx world.WorldState) error {
		var err error
		running, err = advance(ctx, tx, c.conf.GetObjectKey())
		return err
	})
	if err != nil {
		return true, err
	}

	// Watch exactly the Tasks of the running activations.
	c.tasks.SyncKeys(running, true)
	return true, nil
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = (*Controller)(nil).processState

// HandleDirective resolves no directives.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	c.engine.ClearContext()
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
