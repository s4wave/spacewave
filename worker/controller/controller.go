package worker_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/worker"

// Controller implements the Worker controller.
// The Worker processes objects assigned to its peer IDs.
// Manages: Cluster, Task, Pass, Execution
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the execution bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// objKey is the object key (from the config)
	objKey string
	// peerID is the parsed peer id
	// may be empty
	peerID peer.ID

	// objLoop watches the object for changes
	objLoop *world_control.ObjectLoop
	// keypairTrackers watches the list of keypairs for changes.
	keypairTrackers *keyed.Keyed[string, *keypairTracker]
	// objectTrackers manages the list of object tracker routines.
	objectTrackers *keyed.Keyed[string, *objectTracker]
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	peerID, _ := conf.ParsePeerID()
	c := &Controller{
		le:     le,
		bus:    bus,
		conf:   conf,
		objKey: conf.GetObjectKey(),
		peerID: peerID,
	}
	c.objLoop = world_control.NewObjectLoop(
		c.le.WithField("object-loop", "worker-controller"),
		c.objKey,
		c.ProcessState,
	)
	c.keypairTrackers = keyed.NewKeyedWithLogger(c.newKeypairTracker, le)
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
	ctrli, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	cl, ok := ctrli.(*Controller)
	if !ok {
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

// Wake notifies the controller it should re-scan for objects.
func (c *Controller) Wake() {
	c.objLoop.Wake()
}

// Execute executes the controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	c.objectTrackers.SetContext(ctx, true)
	c.keypairTrackers.SetContext(ctx, true)
	return world_control.ExecuteBusObjectLoop(
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
var _ controller.Controller = ((*Controller)(nil))
