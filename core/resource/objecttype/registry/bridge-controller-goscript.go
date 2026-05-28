//go:build goscript

package resource_objecttype_registry

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/sirupsen/logrus"
)

// BridgeController is disabled in GoScript builds because plugin object type
// bridging attaches native world/resource surfaces that are not part of the
// GoScript package graph.
type BridgeController struct{}

// NewBridgeController creates a new BridgeController.
func NewBridgeController(
	le *logrus.Entry,
	b bus.Bus,
	registry *ObjectTypeRegistryResource,
) *BridgeController {
	return &BridgeController{}
}

// GetControllerInfo returns information about the controller.
func (c *BridgeController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"resource/objecttype-registry-bridge",
		controller.MustParseVersion("0.0.1"),
		"objecttype registry bridge controller",
	)
}

// Execute executes the controller.
func (c *BridgeController) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *BridgeController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources held by the controller.
func (c *BridgeController) Close() error {
	return nil
}

func objectTypeRegistryBridgeEnabled() bool {
	return false
}

// _ is a type assertion
var _ controller.Controller = (*BridgeController)(nil)
