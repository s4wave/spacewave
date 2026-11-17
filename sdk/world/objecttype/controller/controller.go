package objecttype_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// LookupObjectTypeFunc looks up an ObjectType by its type ID.
// Returns the ObjectType or nil if not found.
type LookupObjectTypeFunc = func(ctx context.Context, typeID string) (objecttype.ObjectType, error)

// Controller resolves LookupObjectType directives, and LookupObjectTypeMetadata
// directives for the types it finds that carry their metadata.
type Controller struct {
	lookupFunc LookupObjectTypeFunc
}

// NewController creates a controller with a type lookup.
func NewController(lookupFunc LookupObjectTypeFunc) *Controller {
	return &Controller{lookupFunc: lookupFunc}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"objecttype",
		controller.MustParseVersion("1.0.0"),
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
	switch dir := di.GetDirective().(type) {
	case objecttype.LookupObjectType:
		return directive.R(c.resolveObjectType(dir))
	case s4wave_objecttype_registry.LookupObjectTypeMetadata:
		return directive.R(c.resolveMetadata(dir))
	}
	return nil, nil
}

// resolveMetadata resolves a LookupObjectTypeMetadata directive with a copy
// of the metadata the looked-up type carries, if it carries any.
func (c *Controller) resolveMetadata(dir s4wave_objecttype_registry.LookupObjectTypeMetadata) (directive.Resolver, error) {
	typeID := dir.LookupObjectTypeMetadataTypeID()
	if typeID == "" {
		return nil, nil
	}

	return directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		// Look up the type and take the metadata it carries, if any.
		objType, err := c.lookupFunc(ctx, typeID)
		if err != nil {
			return err
		}
		withMetadata, ok := objType.(s4wave_objecttype_registry.MetadataObjectType)
		if !ok || withMetadata.GetObjectTypeMetadata() == nil {
			return nil
		}
		if _, ok := handler.AddValue(withMetadata.GetObjectTypeMetadata().CloneVT()); !ok {
			return ctx.Err()
		}
		return nil
	}), nil
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

var _ controller.Controller = (*Controller)(nil)
