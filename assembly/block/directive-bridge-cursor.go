package assembly_block

import (
	"context"

	"github.com/aperturerobotics/bldr/assembly"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
)

// DirectiveBridgeCursor is a DirectiveBridge with an attached Block cursor.
type DirectiveBridgeCursor struct {
	// a is the directive bridge object
	a *DirectiveBridge
	// ctrlConfig is the controller config
	ctrlConf configset.ControllerConfig
}

// NewDirectiveBridgeCursor builds a new DirectiveBridgeCursor.
func NewDirectiveBridgeCursor(a *DirectiveBridge, ctrlConf configset.ControllerConfig) *DirectiveBridgeCursor {
	return &DirectiveBridgeCursor{a: a, ctrlConf: ctrlConf}
}

// ResolveDirectiveBridgeCursor resolves the config to a controller config.
func ResolveDirectiveBridgeCursor(ctx context.Context, b bus.Bus, a *DirectiveBridge) (*DirectiveBridgeCursor, error) {
	var ctrlConf configset.ControllerConfig
	protoCtrlConf := a.GetControllerConfig()
	if len(protoCtrlConf.GetId()) != 0 {
		var err error
		ctrlConf, err = protoCtrlConf.Resolve(ctx, b)
		if err != nil {
			return nil, err
		}
	}

	return NewDirectiveBridgeCursor(a, ctrlConf), nil
}

// GetBridgeToParent indicates the target is the parent, not the subassembly.
func (c *DirectiveBridgeCursor) GetBridgeToParent() bool {
	return c.a.GetBridgeToParent()
}

// GetControllerConfig returns the directive bridge controller config.
// The controller factory will be looked up on the parent bus.
// The controller must implement DirectiveBridgeController.
// The controller is not run on the bus, but rather as a sub-controller.
// If empty (nil), the directive bridge will be ignored.
func (c *DirectiveBridgeCursor) GetControllerConfig() configset.ControllerConfig {
	if c == nil {
		return nil
	}
	return c.ctrlConf
}

// _ is a type assertion
var _ assembly.DirectiveBridge = ((*DirectiveBridgeCursor)(nil))
