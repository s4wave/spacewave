package identity

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver"
)

// SelectEntityIdController is a controller that resolves SelectEntityId.
type SelectEntityIdController struct {
	// entityID is the entity id to resolve.
	entityID string
}

// NewSelectEntityIdController constructs a new SelectEntityIdController.
func NewSelectEntityIdController(entityID string) *SelectEntityIdController {
	return &SelectEntityIdController{entityID: entityID}
}

// GetControllerInfo returns information about the controller.
func (c *SelectEntityIdController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("identity/select-entity-id", semver.MustParse("0.0.1"), "static select-entity-id resolver")
}

// Execute executes the controller goroutine.
func (c *SelectEntityIdController) Execute(ctx context.Context) error {
	return nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *SelectEntityIdController) Close() error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *SelectEntityIdController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch dir.(type) {
	case SelectEntityId:
		return directive.R(directive.NewValueResolver([]string{c.entityID}), nil)
	}

	return nil, nil
}

// _ is a type assertion
var _ controller.Controller = ((*SelectEntityIdController)(nil))
