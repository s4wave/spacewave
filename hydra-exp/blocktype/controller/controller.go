package blocktype_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/hydra-exp/blocktype"
)

// LookupBlockTypeFunc looks up a BlockType by its type ID.
// Returns the BlockType or nil if not found.
type LookupBlockTypeFunc = func(ctx context.Context, typeID string) (blocktype.BlockType, error)

// Controller resolves LookupBlockType directives.
type Controller struct {
	lookupFunc LookupBlockTypeFunc
}

// NewController creates a controller with a lookup function.
func NewController(lookupFunc LookupBlockTypeFunc) *Controller {
	return &Controller{lookupFunc: lookupFunc}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"blocktype",
		semver.MustParse("1.0.0"),
		"resolves block type lookups",
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
	dir, ok := di.GetDirective().(blocktype.LookupBlockType)
	if !ok {
		return nil, nil
	}

	return directive.R(c.resolveBlockType(dir))
}

// resolveBlockType resolves a LookupBlockType directive.
func (c *Controller) resolveBlockType(dir blocktype.LookupBlockType) (directive.Resolver, error) {
	typeID := dir.LookupBlockTypeID()
	if typeID == "" {
		return nil, nil
	}

	return directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		blockType, err := c.lookupFunc(ctx, typeID)
		if err != nil {
			return err
		}

		if blockType != nil {
			_, _ = handler.AddValue(blockType)
		}

		return nil
	}), nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
