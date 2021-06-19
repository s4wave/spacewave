package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver"
)

// OperationController implements ApplyWorldOp and ApplyObjectOp with a static
// list of callback functions.
type OperationController struct {
	controllerID     string
	engineID         string
	objectID         string
	worldOpHandlers  []ApplyWorldOpFunc
	objectOpHandlers []ApplyObjectOpFunc
}

// OperationControllerVersion is the version of the OperationController.
var OperationControllerVersion = semver.MustParse("0.0.1")

// NewOperationController builds a new operation controller with the handlers.
// controllerID is the id of the operation controller on the bus.
// If engineID or objectID are empty, does not filter those fields.
func NewOperationController(
	controllerID string,
	engineID string,
	objectID string,
	worldOpHandlers []ApplyWorldOpFunc,
	objectOpHandlers []ApplyObjectOpFunc,
) *OperationController {
	return &OperationController{
		controllerID: controllerID,
		engineID:     engineID, objectID: objectID,
		worldOpHandlers:  worldOpHandlers,
		objectOpHandlers: objectOpHandlers,
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (c *OperationController) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) (directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case ApplyWorldOp:
		return c.resolveApplyWorldOp(ctx, di, d)
	case ApplyObjectOp:
		return c.resolveApplyObjectOp(ctx, di, d)
	}
	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *OperationController) GetControllerInfo() controller.Info {
	return controller.NewInfo(
		c.controllerID,
		OperationControllerVersion,
		"world op handlers set",
	)
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *OperationController) Execute(ctx context.Context) error {
	return nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *OperationController) Close() error {
	return nil
}

// resolveApplyWorldOp returns a resolver for the ApplyWorldOp directive.
func (c *OperationController) resolveApplyWorldOp(
	ctx context.Context,
	di directive.Instance,
	d ApplyWorldOp,
) (directive.Resolver, error) {
	if len(c.worldOpHandlers) == 0 {
		return nil, nil
	}
	if c.engineID != "" && d.ApplyWorldOpEngineID() != c.engineID {
		return nil, nil
	}
	return NewApplyWorldOpResolver(c.worldOpHandlers), nil
}

// resolveApplyObjectOp returns a resolver for the ApplyObjectOp directive.
func (c *OperationController) resolveApplyObjectOp(
	ctx context.Context,
	di directive.Instance,
	d ApplyObjectOp,
) (directive.Resolver, error) {
	if len(c.objectOpHandlers) == 0 {
		return nil, nil
	}
	if c.engineID != "" && d.ApplyObjectOpEngineID() != c.engineID {
		return nil, nil
	}
	if c.objectID != "" && d.ApplyObjectOpObjectID() != c.objectID {
		return nil, nil
	}
	return NewApplyObjectOpResolver(c.objectOpHandlers), nil
}

// _ is a type assertion
var _ controller.Controller = ((*OperationController)(nil))
