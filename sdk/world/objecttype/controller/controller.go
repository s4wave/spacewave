package objecttype_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// LookupObjectTypeFunc looks up an ObjectType by its type ID.
// Returns the ObjectType or nil if not found.
type LookupObjectTypeFunc = func(ctx context.Context, typeID string) (objecttype.ObjectType, error)

// Controller resolves LookupObjectType directives.
type Controller struct {
	lookupFunc LookupObjectTypeFunc
}

// NewController creates a controller with a lookup function.
func NewController(lookupFunc LookupObjectTypeFunc) *Controller {
	return &Controller{lookupFunc: lookupFunc}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"objecttype",
		semver.MustParse("1.0.0"),
		"resolves object type lookups",
	)
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// Close closes the controller.
func (c *Controller) Close() error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(objecttype.LookupObjectType)
	if !ok {
		return nil, nil
	}

	return directive.R(c.resolveObjectType(dir))
}

// resolveObjectType resolves a LookupObjectType directive.
func (c *Controller) resolveObjectType(dir objecttype.LookupObjectType) (directive.Resolver, error) {
	typeID := dir.LookupObjectTypeID()
	if typeID == "" {
		return nil, nil
	}

	return directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		objType, err := c.lookupFunc(ctx, typeID)
		if err != nil {
			return err
		}

		if objType != nil {
			if _, ok := handler.AddValue(objType); !ok {
				return ctx.Err()
			}
		}

		return nil
	}), nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
