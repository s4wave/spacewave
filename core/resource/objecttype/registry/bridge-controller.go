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
	resource "github.com/s4wave/spacewave/bldr/resource"
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
	reg := c.registry.lookupRegistration(typeID)
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
	reg *objectTypeRegistration
}

// newBridgeResolver creates a new bridgeResolver.
func newBridgeResolver(
	le *logrus.Entry,
	b bus.Bus,
	reg *objectTypeRegistration,
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
		if r.reg.attached != nil {
			return r.invokeAttached(ctx, objectKey, engine)
		}
		return r.invokePlugin(ctx, objectKey, engine)
	}
	ot := objecttype.NewObjectType(r.reg.registration.GetTypeId(), factory)
	handler.AddValue(ot)
	return nil
}

// invokeAttached creates an ObjectType proxy through a caller-attached handler.
func (r *bridgeResolver) invokeAttached(
	ctx context.Context,
	objectKey string,
	engine world.Engine,
) (srpc.Invoker, func(), error) {
	invoker := newAttachedObjectTypeInvoker(r.reg.attached, r.reg.registration, objectKey, engine, r.b, r.le)
	if err := invoker.connect(ctx); err != nil {
		return nil, nil, err
	}
	return invoker, invoker.Close, nil
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
	invoker := newPluginObjectTypeInvoker(resourceClientCtx, r.reg.registration, objectKey, engine, r.b, r.le)
	if err := invoker.connect(ctx); err != nil {
		return nil, nil, err
	}
	return invoker, invoker.Close, nil
}

// pluginResourceSession drives one nested ResourceClient generation for a
// proxied ObjectType. It owns the resource client, the optional world-engine
// attachment, the root and child references, and the child SRPC client.
type pluginResourceSession struct {
	le        *logrus.Entry
	b         bus.Bus
	reg       *s4wave_objecttype_registry.ObjectTypeRegistration
	objectKey string
	engine    world.Engine

	// openResources opens the ResourceClient generation. The alive callback
	// reports whether the generation is still usable; nil means always.
	openResources func(ctx context.Context) (*resource_client.Client, func(), func() bool, error)
	// detachCtx outlives one request and is used to detach the engine.
	detachCtx context.Context

	mtx sync.Mutex

	resources        *resource_client.Client
	releaseResources func()
	alive            func() bool
	engineResourceID uint32
	rootRef          resource_client.ResourceRef
	childRef         resource_client.ResourceRef
	childClient      srpc.Client
}

// connectLocked opens a fresh session, replacing any current one.
func (s *pluginResourceSession) connectLocked(ctx context.Context) error {
	s.resetLocked()
	client, releaseResources, alive, err := s.openResources(ctx)
	if err != nil {
		return err
	}
	s.resources = client
	s.releaseResources = releaseResources
	s.alive = alive

	if s.engine != nil {
		lookupOp := space_world_optypes.BuildSpaceLookupOp(s.b, s.le, "")
		engineRes := resource_world.NewEngineResource(s.le, s.b, s.engine, lookupOp, nil)
		s.engineResourceID, err = client.AttachResource(ctx, "world-engine", engineRes.GetMux())
		if err != nil {
			s.resetLocked()
			return err
		}
	}

	s.rootRef = client.AccessRootResource()
	rootClient, err := s.rootRef.GetClient()
	if err != nil {
		s.resetLocked()
		return err
	}

	handlerSvc := s4wave_objecttype_registry.NewSRPCObjectTypeHandlerServiceClient(rootClient)
	resp, err := handlerSvc.InvokeObjectType(ctx, &s4wave_objecttype_registry.InvokeObjectTypeRequest{
		TypeId:                   s.reg.GetTypeId(),
		ObjectKey:                s.objectKey,
		AttachedEngineResourceId: s.engineResourceID,
	})
	if err != nil {
		s.resetLocked()
		return err
	}

	s.childRef = client.CreateResourceReference(resp.GetResourceId())
	s.childClient, err = s.childRef.GetClient()
	if err != nil {
		s.resetLocked()
		return err
	}
	return nil
}

// resetLocked releases the child, root, engine attachment, and client
// generation.
func (s *pluginResourceSession) resetLocked() {
	if s.childRef != nil {
		s.childRef.Release()
	}
	if s.rootRef != nil {
		s.rootRef.Release()
	}
	if s.resources != nil {
		if s.engineResourceID != 0 {
			_ = s.resources.DetachResource(s.detachCtx, s.engineResourceID)
		}
		if s.releaseResources != nil {
			s.releaseResources()
		} else {
			s.resources.Release()
		}
	}
	s.resources = nil
	s.releaseResources = nil
	s.alive = nil
	s.engineResourceID = 0
	s.rootRef = nil
	s.childRef = nil
	s.childClient = nil
}

// connect replaces the current session with a fresh one.
func (s *pluginResourceSession) connect(ctx context.Context) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.connectLocked(ctx)
}

// close releases the current session.
func (s *pluginResourceSession) close() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.resetLocked()
}

// reset drops the current session so the next use reconnects.
func (s *pluginResourceSession) reset() {
	s.close()
}

// peekChildClient returns the connected child client without reconnecting.
func (s *pluginResourceSession) peekChildClient() srpc.Client {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.childClient
}

// currentClient returns a live child client, reconnecting when the session
// was released or the plugin runtime dropped it.
func (s *pluginResourceSession) currentClient(ctx context.Context) (srpc.Client, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.childClient != nil && (s.alive == nil || s.alive()) {
		return s.childClient, nil
	}
	if err := s.connectLocked(ctx); err != nil {
		return nil, err
	}
	return s.childClient, nil
}

// attachedObjectTypeInvoker holds one nested ResourceClient generation. It is
// invalidated when the caller generation that supplied the handler ends.
type attachedObjectTypeInvoker struct {
	session pluginResourceSession
}

// newAttachedObjectTypeInvoker creates an invoker over a caller-attached
// handler.
func newAttachedObjectTypeInvoker(
	handler *attachedObjectTypeHandler,
	reg *s4wave_objecttype_registry.ObjectTypeRegistration,
	objectKey string,
	engine world.Engine,
	b bus.Bus,
	le *logrus.Entry,
) *attachedObjectTypeInvoker {
	i := &attachedObjectTypeInvoker{}
	i.session = pluginResourceSession{
		le:        le,
		b:         b,
		reg:       reg,
		objectKey: objectKey,
		engine:    engine,
		detachCtx: handler.ctx,
		openResources: func(_ context.Context) (*resource_client.Client, func(), func() bool, error) {
			client, err := resource_client.NewClient(
				handler.ctx,
				resource.NewSRPCResourceServiceClient(handler.client),
			)
			if err != nil {
				return nil, nil, nil, err
			}
			return client, client.Release, nil, nil
		},
	}
	return i
}

// InvokeMethod invokes a method on the attached handler child resource.
func (i *attachedObjectTypeInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	childClient := i.session.peekChildClient()
	if childClient == nil {
		return false, resource.ErrResourceOrClientReleased
	}
	return srpc.NewClientInvoker(childClient).InvokeMethod(serviceID, methodID, strm)
}

func (i *attachedObjectTypeInvoker) connect(ctx context.Context) error {
	return i.session.connect(ctx)
}

// Close releases the child, root, engine attachment, and client generation.
func (i *attachedObjectTypeInvoker) Close() {
	i.session.close()
}

// pluginObjectTypeInvoker connects to the source plugin and proxies method
// invokes, reconnecting when the plugin runtime drops the session.
type pluginObjectTypeInvoker struct {
	session pluginResourceSession
}

// newPluginObjectTypeInvoker creates an invoker that connects to the plugin
// named by reg and reconnects on released sessions.
func newPluginObjectTypeInvoker(
	resourceClientCtx context.Context,
	reg *s4wave_objecttype_registry.ObjectTypeRegistration,
	objectKey string,
	engine world.Engine,
	b bus.Bus,
	le *logrus.Entry,
) *pluginObjectTypeInvoker {
	i := &pluginObjectTypeInvoker{}
	i.session = pluginResourceSession{
		le:        le,
		b:         b,
		reg:       reg,
		objectKey: objectKey,
		engine:    engine,
		detachCtx: resourceClientCtx,
		openResources: func(_ context.Context) (*resource_client.Client, func(), func() bool, error) {
			resources, err := s4wave_plugin.ConnectPluginResources(resourceClientCtx, b, reg.GetPluginId())
			if err != nil {
				return nil, nil, nil, err
			}
			alive := func() bool {
				select {
				case <-resources.Client.Done():
					return false
				default:
					return true
				}
			}
			return resources.Client, resources.Release, alive, nil
		},
	}
	return i
}

// InvokeMethod invokes a method on the plugin child resource, retrying once
// through a fresh session when the plugin dropped the old one.
func (i *pluginObjectTypeInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	childClient, err := i.session.currentClient(strm.Context())
	if err != nil {
		return false, err
	}
	found, err := srpc.NewClientInvoker(childClient).InvokeMethod(serviceID, methodID, strm)
	if err == nil || !shouldReconnectPluginInvoke(strm.Context(), err) {
		return found, err
	}
	i.session.reset()
	childClient, retryErr := i.session.currentClient(strm.Context())
	if retryErr != nil {
		return false, retryErr
	}
	return srpc.NewClientInvoker(childClient).InvokeMethod(serviceID, methodID, strm)
}

func (i *pluginObjectTypeInvoker) connect(ctx context.Context) error {
	return i.session.connect(ctx)
}

// Close releases the currently connected plugin ObjectType resource.
func (i *pluginObjectTypeInvoker) Close() {
	i.session.close()
}

// shouldReconnectPluginInvoke reports whether the invoke failed because the
// plugin dropped the resource session rather than the call itself.
func shouldReconnectPluginInvoke(ctx context.Context, err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "resource not found") ||
		strings.Contains(msg, "invalid resource id") ||
		strings.Contains(msg, "resource or client was released") {
		return true
	}
	if !errors.Is(err, context.Canceled) {
		return false
	}
	return ctx.Err() == nil
}

// _ is a type assertion
var _ controller.Controller = (*BridgeController)(nil)

// _ is a type assertion
var _ directive.Resolver = (*bridgeResolver)(nil)
