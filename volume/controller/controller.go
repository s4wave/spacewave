package volume_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	volume "github.com/aperturerobotics/hydra/volume"
	"github.com/sirupsen/logrus"
)

// Controller implements a common volume controller.
// The controller looks up the peer, acquires its identity, constructs the
// transport, and manages the lifecycle of dialing and accepting links.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// ctor is the constructor
	ctor volume.Constructor
	// volumeCh contains the controlled volume
	volumeCh chan volume.Volume
	// controllerInfo contains the controller info
	controllerInfo controller.Info
}

// NewController constructs a new volume controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	info controller.Info,
	ctor volume.Constructor,
) *Controller {
	return &Controller{
		le:             le,
		bus:            bus,
		ctor:           ctor,
		controllerInfo: info,
		volumeCh:       make(chan volume.Volume, 1),
	}
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// Construct the volume.
	// This will query the peer private key.
	le := c.le
	v, err := c.ctor(ctx, le)
	if err != nil {
		return err
	}
	defer v.Close()

	c.volumeCh <- v

	// volume is ready, process directives.
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) (directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case peer.GetPeer:
		return newGetPeerResolver(c, d), nil
	}

	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() controller.Info {
	return c.controllerInfo
}

// GetVolume returns the controlled volume.
// This may wait for the volume to be ready.
func (c *Controller) GetVolume(ctx context.Context) (volume.Volume, error) {
	var r volume.Volume
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r = <-c.volumeCh:
		c.volumeCh <- r
	}

	return r, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ volume.Controller = ((*Controller)(nil))
