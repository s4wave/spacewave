//go:build goscript

package resource_worldop_registry

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/sirupsen/logrus"
)

// bridgeControllerID is the controller ID.
const bridgeControllerID = "resource/worldop-registry-bridge"

// bridgeControllerVersion is the controller version.
var bridgeControllerVersion = controller.MustParseVersion("0.0.1")

// WorldOpRegistryBridgeController is disabled in GoScript builds because
// plugin world-op bridging attaches native world/resource surfaces that are
// not part of the GoScript package graph.
type WorldOpRegistryBridgeController struct{}

// NewWorldOpRegistryBridgeController creates a new WorldOpRegistryBridgeController.
func NewWorldOpRegistryBridgeController(
	le *logrus.Entry,
	b bus.Bus,
	registry *WorldOpRegistryResource,
) *WorldOpRegistryBridgeController {
	return &WorldOpRegistryBridgeController{}
}

// GetControllerInfo returns controller info.
func (c *WorldOpRegistryBridgeController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(bridgeControllerID, bridgeControllerVersion, "worldop registry bridge controller")
}

// Execute executes the controller.
func (c *WorldOpRegistryBridgeController) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *WorldOpRegistryBridgeController) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources held by the controller.
func (c *WorldOpRegistryBridgeController) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*WorldOpRegistryBridgeController)(nil)
