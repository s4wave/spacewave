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

// LookupMetadataFunc returns an object type's metadata, or nil if it has none.
type LookupMetadataFunc = func(typeID string) *s4wave_objecttype_registry.ObjectTypeMetadata

// Controller resolves LookupObjectType directives, and LookupObjectTypeMetadata
// directives for the types its metadata lookup knows.
type Controller struct {
	lookupFunc   LookupObjectTypeFunc
	metadataFunc LookupMetadataFunc
}

// NewController creates a controller with a type lookup. It resolves no
// metadata.
func NewController(lookupFunc LookupObjectTypeFunc) *Controller {
	return &Controller{lookupFunc: lookupFunc}
}

// NewControllerWithMetadata creates a controller with a type lookup and a
// metadata lookup for the same types.
func NewControllerWithMetadata(lookupFunc LookupObjectTypeFunc, metadataFunc LookupMetadataFunc) *Controller {
	return &Controller{lookupFunc: lookupFunc, metadataFunc: metadataFunc}
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
		return c.resolveMetadata(dir), nil
	}
	return nil, nil
}

// resolveMetadata resolves a LookupObjectTypeMetadata directive with the
// metadata lookup's copy. Compiled metadata never changes, so it is one value.
func (c *Controller) resolveMetadata(dir s4wave_objecttype_registry.LookupObjectTypeMetadata) []directive.Resolver {
	if c.metadataFunc == nil {
		return nil
	}
	metadata := c.metadataFunc(dir.LookupObjectTypeMetadataTypeID())
	if metadata == nil {
		return nil
	}
	return []directive.Resolver{directive.NewValueResolver([]s4wave_objecttype_registry.LookupObjectTypeMetadataValue{metadata})}
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
