package link_holdopen_controller

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/net/link"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bifrost/link/hold-open"

// Controller is the greedy Link hold-open controller.
// It adds a reference to all EstablishLink directives with at least one Link.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus

	// mtx guards cleanupRefs
	mtx sync.Mutex
	// cleanupRefs are released when the controller is closed
	cleanupRefs []directive.Reference
}

// NewController constructs a new peer controller.
// If privKey is nil, one will be generated.
func NewController(b bus.Bus, le *logrus.Entry) (*Controller, error) {
	return &Controller{
		le:  le,
		bus: b,
	}, nil
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case link.EstablishLinkWithPeer:
		c.handleEstablishLink(ctx, di, d)
	}
	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"link hold-open controller",
	)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	c.mtx.Lock()
	refs := c.cleanupRefs
	c.cleanupRefs = nil
	c.mtx.Unlock()
	for _, ref := range refs {
		ref.Release()
	}
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
