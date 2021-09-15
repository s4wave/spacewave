package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver"
)

// LookupOpController implements LookupWorldOp on a bus.
type LookupOpController struct {
	controllerID string
	engineID     string
	lookupOp     LookupOp
}

// LookupOpControllerVersion is the version of the LookupOpController.
var LookupOpControllerVersion = semver.MustParse("0.0.1")

// NewLookupOpController builds a new operation controller with the handlers.
// controllerID is the id of the operation controller on the bus.
// if engineID is empty, does not filter that field
func NewLookupOpController(
	controllerID string,
	engineID string,
	lookupOp LookupOp,
) *LookupOpController {
	return &LookupOpController{
		controllerID: controllerID,
		engineID:     engineID,
		lookupOp:     lookupOp,
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (c *LookupOpController) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) (directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case LookupWorldOp:
		return c.resolveLookupWorldOp(ctx, di, d)
	}
	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *LookupOpController) GetControllerInfo() controller.Info {
	return controller.NewInfo(
		c.controllerID,
		LookupOpControllerVersion,
		"world op lookup",
	)
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *LookupOpController) Execute(ctx context.Context) error {
	return nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *LookupOpController) Close() error {
	return nil
}

// resolveLookupWorldOp returns a resolver for the LookupWorldOp directive.
func (c *LookupOpController) resolveLookupWorldOp(
	ctx context.Context,
	di directive.Instance,
	d LookupWorldOp,
) (directive.Resolver, error) {
	if c.lookupOp == nil {
		return nil, nil
	}
	if c.engineID != "" && d.LookupWorldOpEngineID() != c.engineID {
		return nil, nil
	}
	return NewLookupWorldOpResolver(c.lookupOp), nil
}

// _ is a type assertion
var _ controller.Controller = ((*LookupOpController)(nil))
