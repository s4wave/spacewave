package plugin_objecttype

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// ControllerID is the controller ID.
const ControllerID = "plugin/objecttype"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "resolves LookupObjectType directives for plugin-provided types"

// Controller resolves LookupObjectType directives for plugin-provided types.
// When a plugin loads, it creates this controller with its ObjectType map
// and adds it to the bus. When the plugin unloads, the controller is removed,
// cleaning up registrations automatically.
type Controller struct {
	// types maps type ID to ObjectType.
	types map[string]objecttype.ObjectType
}

// NewController creates a new plugin ObjectType controller.
func NewController(types map[string]objecttype.ObjectType) *Controller {
	return &Controller{types: types}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		controllerDescrip,
	)
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(objecttype.LookupObjectType)
	if !ok {
		return nil, nil
	}

	return c.resolveLookupObjectType(dir)
}

// resolveLookupObjectType resolves a LookupObjectType directive.
func (c *Controller) resolveLookupObjectType(dir objecttype.LookupObjectType) ([]directive.Resolver, error) {
	tid := dir.LookupObjectTypeID()
	if tid == "" {
		return nil, nil
	}

	ot, ok := c.types[tid]
	if !ok {
		return nil, nil
	}

	return directive.R(directive.NewValueResolver([]objecttype.ObjectType{ot}), nil)
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
