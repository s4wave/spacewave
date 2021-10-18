package assembly

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
)

// DirectiveBridge connects two Bus by applying Directives to the other.
type DirectiveBridge interface {
	// GetBridgeToParent indicates the target is the parent, not the subassembly.
	GetBridgeToParent() bool
	// GetControllerConfig returns the directive bridge controller config.
	// The controller factory will be looked up on the parent bus.
	// The controller must implement DirectiveBridgeController.
	// The controller is not run on the bus, but rather as a sub-controller.
	// If empty (nil), the directive bridge will be ignored.
	GetControllerConfig() configset.ControllerConfig
}

// DirectiveBridgeController bridges directives to a target bus.
type DirectiveBridgeController interface {
	// Controller is the controllerbus controller interface.
	controller.Controller
	// SetDirectiveBridgeTarget sets the target bus.
	// called before HandleDirective and Execute
	SetDirectiveBridgeTarget(b bus.Bus)
}
