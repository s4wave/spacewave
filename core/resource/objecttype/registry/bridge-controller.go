package resource_objecttype_registry

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/world"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_plugin "github.com/s4wave/spacewave/sdk/plugin"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// BridgeController resolves LookupObjectType directives for registry-registered
// types by proxying to the source TS plugin.
type BridgeController struct {
	le       *logrus.Entry
	b        bus.Bus
	registry *ObjectTypeRegistryResource
}

// NewBridgeController creates a new BridgeController.
func NewBridgeController(
	le *logrus.Entry,
	b bus.Bus,
	registry *ObjectTypeRegistryResource,
) *BridgeController {
	return &BridgeController{
		le:       le,
		b:        b,
		registry: registry,
	}
}

// GetControllerInfo returns information about the controller.
func (c *BridgeController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"resource/objecttype-registry-bridge",
		semver.MustParse("0.0.1"),
		"resolves LookupObjectType for plugin-registered types",
	)
}

// Execute executes the controller.
func (c *BridgeController) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *BridgeController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(objecttype.LookupObjectType)
	if !ok {
		return nil, nil
	}
	typeID := dir.LookupObjectTypeID()
	if typeID == "" {
		return nil, nil
	}
	reg := c.registry.LookupRegistration(typeID)
	if reg == nil {
		return nil, nil
	}
	return directive.R(newBridgeResolver(c.le, c.b, reg), nil)
}

// Close releases any resources held by the controller.
func (c *BridgeController) Close() error {
	return nil
}

// bridgeResolver resolves a LookupObjectType directive by creating a proxy
// ObjectType that connects to the TS plugin.
type bridgeResolver struct {
	le  *logrus.Entry
	b   bus.Bus
	reg *s4wave_objecttype_registry.ObjectTypeRegistration
}

// newBridgeResolver creates a new bridgeResolver.
func newBridgeResolver(
	le *logrus.Entry,
	b bus.Bus,
	reg *s4wave_objecttype_registry.ObjectTypeRegistration,
) *bridgeResolver {
	return &bridgeResolver{
		le:  le,
		b:   b,
		reg: reg,
	}
}

// Resolve resolves the values, emitting them to the handler.
func (r *bridgeResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	factory := func(
		ctx context.Context,
		le *logrus.Entry,
		b bus.Bus,
		engine world.Engine,
		ws world.WorldState,
		objectKey string,
	) (srpc.Invoker, func(), error) {
		return r.invokePlugin(ctx, objectKey, engine)
	}
	ot := objecttype.NewObjectType(r.reg.GetTypeId(), factory)
	handler.AddValue(ot)
	return nil
}

// invokePlugin connects to the source plugin and creates a proxy invoker.
// If engine is non-nil, it is attached as a resource so the TS handler
// can access the world via getAttachedRef(engineResourceId).
func (r *bridgeResolver) invokePlugin(
	ctx context.Context,
	objectKey string,
	engine world.Engine,
) (srpc.Invoker, func(), error) {
	resources, err := s4wave_plugin.ConnectPluginResources(ctx, r.b, r.reg.GetPluginId())
	if err != nil {
		return nil, nil, err
	}

	// Attach world engine as a resource if available.
	var engineResourceID uint32
	if engine != nil {
		engineRes := resource_world.NewEngineResource(r.le, r.b, engine, nil, nil)
		engineResourceID, err = resources.Client.AttachResource(ctx, "world-engine", engineRes.GetMux())
		if err != nil {
			resources.Release()
			return nil, nil, err
		}
	}

	rootRef := resources.Client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		resources.Release()
		return nil, nil, err
	}

	handlerSvc := s4wave_objecttype_registry.NewSRPCObjectTypeHandlerServiceClient(rootClient)
	resp, err := handlerSvc.InvokeObjectType(ctx, &s4wave_objecttype_registry.InvokeObjectTypeRequest{
		TypeId:           r.reg.GetTypeId(),
		ObjectKey:        objectKey,
		EngineResourceId: engineResourceID,
	})
	if err != nil {
		rootRef.Release()
		resources.Release()
		return nil, nil, err
	}

	childRef := resources.Client.CreateResourceReference(resp.GetResourceId())
	childClient, err := childRef.GetClient()
	if err != nil {
		childRef.Release()
		rootRef.Release()
		resources.Release()
		return nil, nil, err
	}

	invoker := srpc.NewClientInvoker(childClient)
	cleanup := func() {
		childRef.Release()
		rootRef.Release()
		resources.Release()
	}
	return invoker, cleanup, nil
}

// _ is a type assertion
var _ controller.Controller = (*BridgeController)(nil)

// _ is a type assertion
var _ directive.Resolver = (*bridgeResolver)(nil)
