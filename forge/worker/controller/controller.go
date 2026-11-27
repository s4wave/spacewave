package worker_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/db/block"
	world_control "github.com/s4wave/spacewave/db/world/control"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/worker"

// Controller implements the Worker controller. The Worker processes the
// objects assigned to its keypairs: Clusters, Tasks, Passes and Executions.
type Controller struct {
	// le is the root logger.
	le *logrus.Entry
	// bus is the execution bus.
	bus bus.Bus
	// conf is the config.
	conf *Config
	// objKey is the object key (from the config).
	objKey string
	// peerID is the parsed peer id.
	// may be empty
	peerID peer.ID

	// objLoop watches the object for changes.
	objLoop *world_control.WatchLoop
	// objectTrackers manages the list of object tracker routines.
	objectTrackers *keyed.Keyed[string, *objectTracker]
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	// Parse the configured peer identity before assembling controller state.
	peerID, _ := conf.ParsePeerID()
	c := &Controller{
		le:     le,
		bus:    bus,
		conf:   conf,
		objKey: conf.GetObjectKey(),
		peerID: peerID,
	}
	c.objLoop = world_control.NewWatchLoop(
		c.le.WithField("object-loop", "worker-controller"),
		c.objKey,
		c.ProcessState,
	)
	c.objectTrackers = keyed.NewKeyedWithLogger(c.newObjectTracker, le)
	return c
}

// StartControllerWithConfig starts a controller with a config.
// Waits for the controller to start.
// Returns a Release function to close the controller when done.
func StartControllerWithConfig(
	ctx context.Context,
	b bus.Bus,
	conf *Config,
) (*Controller, directive.Reference, error) {
	// Wait for the configured controller to start on the bus.
	ctrli, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	// Require the loaded controller to have the expected type.
	cl, ok := ctrli.(*Controller)
	if !ok {
		ctrlRef.Release()
		return nil, nil, block.ErrUnexpectedType
	}
	return cl, ctrlRef, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"worker controller",
	)
}

// Execute executes the controller.
func (c *Controller) Execute(rctx context.Context) error {
	// Bind the worker trackers to the controller's execution lifetime.
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// Start the object trackers before running the World watch loop.
	c.objectTrackers.SetContext(ctx, true)
	return world_control.ExecuteBusWatchLoop(
		ctx,
		c.bus,
		c.conf.GetEngineId(),
		true,
		c.objLoop,
	)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
