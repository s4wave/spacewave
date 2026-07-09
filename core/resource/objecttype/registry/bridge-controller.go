package resource_objecttype_registry

import (
	"context"
	"strings"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
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
		controller.MustParseVersion("0.0.1"),
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
// can access the world via getAttachedRef(attachedEngineResourceId).
func (r *bridgeResolver) invokePlugin(
	ctx context.Context,
	objectKey string,
	engine world.Engine,
) (srpc.Invoker, func(), error) {
	resourceCtx := resource_server.GetResourceClientContext(ctx)
	resourceClientCtx := ctx
	if resourceCtx != nil {
		resourceClientCtx = resourceCtx.Context()
	}
	invoker := &pluginObjectTypeInvoker{
		le:                r.le,
		b:                 r.b,
		reg:               r.reg,
		objectKey:         objectKey,
		engine:            engine,
		resourceClientCtx: resourceClientCtx,
	}
	if err := invoker.connect(ctx); err != nil {
		return nil, nil, err
	}
	return invoker, invoker.Close, nil
}

type pluginObjectTypeInvoker struct {
	le                *logrus.Entry
	b                 bus.Bus
	reg               *s4wave_objecttype_registry.ObjectTypeRegistration
	objectKey         string
	engine            world.Engine
	resourceClientCtx context.Context

	mtx              sync.Mutex
	resources        *s4wave_plugin.PluginResources
	engineResourceID uint32
	rootRef          resource_client.ResourceRef
	childRef         resource_client.ResourceRef
	childClient      srpc.Client
}

func (i *pluginObjectTypeInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	childClient, err := i.currentClient(strm.Context())
	if err != nil {
		return false, err
	}
	found, err := srpc.NewClientInvoker(childClient).InvokeMethod(serviceID, methodID, strm)
	if err == nil || !isStalePluginResourceError(err) {
		return found, err
	}
	i.reset()
	childClient, retryErr := i.currentClient(strm.Context())
	if retryErr != nil {
		return false, retryErr
	}
	return srpc.NewClientInvoker(childClient).InvokeMethod(serviceID, methodID, strm)
}

func (i *pluginObjectTypeInvoker) currentClient(ctx context.Context) (srpc.Client, error) {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	if i.childClient != nil && i.resources != nil {
		select {
		case <-i.resources.Client.Done():
		default:
			return i.childClient, nil
		}
	}
	i.releaseLocked()
	if err := i.connectLocked(ctx); err != nil {
		return nil, err
	}
	return i.childClient, nil
}

func (i *pluginObjectTypeInvoker) connect(ctx context.Context) error {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	return i.connectLocked(ctx)
}

func (i *pluginObjectTypeInvoker) connectLocked(ctx context.Context) error {
	resources, err := s4wave_plugin.ConnectPluginResources(i.resourceClientCtx, i.b, i.reg.GetPluginId())
	if err != nil {
		return err
	}

	// Plugin ObjectTypes commit through an attached world engine. If the
	// plugin runtime reconnects, the attachment belongs to the old resource
	// client and must be recreated with the child ObjectType resource.
	var engineResourceID uint32
	if i.engine != nil {
		lookupOp := space_world_optypes.BuildSpaceLookupOp(i.b, i.le, "")
		engineRes := resource_world.NewEngineResource(i.le, i.b, i.engine, lookupOp, nil)
		engineResourceID, err = resources.Client.AttachResource(ctx, "world-engine", engineRes.GetMux())
		if err != nil {
			resources.Release()
			return err
		}
	}

	rootRef := resources.Client.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		if engineResourceID != 0 {
			_ = resources.Client.DetachResource(i.resourceClientCtx, engineResourceID)
		}
		resources.Release()
		return errors.Wrapf(
			err,
			"invoke objecttype plugin_id=%s capability=engine attached_root_id=%d path=InvokeObjectType type_id=%s object_key=%s",
			i.reg.GetPluginId(),
			engineResourceID,
			i.reg.GetTypeId(),
			i.objectKey,
		)
	}

	handlerSvc := s4wave_objecttype_registry.NewSRPCObjectTypeHandlerServiceClient(rootClient)
	resp, err := handlerSvc.InvokeObjectType(ctx, &s4wave_objecttype_registry.InvokeObjectTypeRequest{
		TypeId:                   i.reg.GetTypeId(),
		ObjectKey:                i.objectKey,
		AttachedEngineResourceId: engineResourceID,
	})
	if err != nil {
		rootRef.Release()
		if engineResourceID != 0 {
			_ = resources.Client.DetachResource(i.resourceClientCtx, engineResourceID)
		}
		resources.Release()
		return err
	}

	childRef := resources.Client.CreateResourceReference(resp.GetResourceId())
	childClient, err := childRef.GetClient()
	if err != nil {
		childRef.Release()
		rootRef.Release()
		if engineResourceID != 0 {
			_ = resources.Client.DetachResource(i.resourceClientCtx, engineResourceID)
		}
		resources.Release()
		return err
	}

	i.resources = resources
	i.engineResourceID = engineResourceID
	i.rootRef = rootRef
	i.childRef = childRef
	i.childClient = childClient
	return nil
}

// Close releases the currently connected plugin ObjectType resource.
func (i *pluginObjectTypeInvoker) Close() {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	i.releaseLocked()
}

func (i *pluginObjectTypeInvoker) reset() {
	i.mtx.Lock()
	defer i.mtx.Unlock()
	i.releaseLocked()
}

func (i *pluginObjectTypeInvoker) releaseLocked() {
	if i.childRef != nil {
		i.childRef.Release()
	}
	if i.rootRef != nil {
		i.rootRef.Release()
	}
	if i.resources != nil {
		if i.engineResourceID != 0 {
			_ = i.resources.Client.DetachResource(i.resourceClientCtx, i.engineResourceID)
		}
		i.resources.Release()
	}
	i.resources = nil
	i.engineResourceID = 0
	i.rootRef = nil
	i.childRef = nil
	i.childClient = nil
}

func isStalePluginResourceError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "resource not found") ||
		strings.Contains(msg, "invalid resource id") ||
		strings.Contains(msg, "resource or client was released")
}

// _ is a type assertion
var _ controller.Controller = (*BridgeController)(nil)

// _ is a type assertion
var _ directive.Resolver = (*bridgeResolver)(nil)
