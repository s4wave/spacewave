package resource_objecttype_registry

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

func TestBridgeResolverKeepsPluginResourceClientAfterRequestContextCancel(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	childReleased := make(chan struct{}, 1)
	pluginRoot := srpc.NewMux()
	if err := s4wave_objecttype_registry.SRPCRegisterObjectTypeHandlerService(pluginRoot, &testObjectTypeHandler{childReleased: childReleased}); err != nil {
		t.Fatal(err)
	}
	pluginResourceMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(pluginRoot).Register(pluginResourceMux); err != nil {
		t.Fatal(err)
	}
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginResourceMux)))

	rel, err := tb.Bus.AddController(ctx, &testPluginLoadController{client: pluginClient}, nil)
	if err != nil {
		t.Fatalf("AddController: %v", err)
	}
	defer rel()
	registry := NewObjectTypeRegistryResource()
	registry.registrations[1] = &objectTypeRegistration{registration: &s4wave_objecttype_registry.ObjectTypeRegistration{
		TypeId:         "test/type",
		RegistrationId: 1,
		PluginId:       "test-plugin",
	}}
	ctrl := NewBridgeController(le, tb.Bus, registry)
	rel, err = tb.Bus.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatalf("AddController bridge: %v", err)
	}
	defer rel()

	ownerCtx, ownerCancel := context.WithCancel(ctx)
	defer ownerCancel()
	requestCtx, requestCancel := context.WithCancel(resource_server.WithResourceClientContext(ctx, &testResourceClientContext{ctx: ownerCtx}))

	ot, ref, err := objecttype.ExLookupObjectType(ctx, tb.Bus, "test/type")
	if err != nil {
		t.Fatalf("ExLookupObjectType: %v", err)
	}
	if ref != nil {
		defer ref.Release()
	}
	if ot == nil {
		t.Fatalf("expected object type")
	}
	invoker, cleanup, err := ot.GetFactory()(requestCtx, le, tb.Bus, tb.Engine, nil, "test/object")
	if err != nil {
		t.Fatalf("object type factory: %v", err)
	}

	requestCancel()

	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))
	if err := client.ExecCall(ctx, "test.Child", "Ping", &testPingMessage{}, &testPingMessage{}); err != nil {
		t.Fatalf("child resource call after request context cancel: %v", err)
	}
	readTx, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	if _, err := world.MustGetObject(ctx, readTx, "test/objecttype-seed"); err != nil {
		t.Fatalf("seeded object was not committed through attached engine: %v", err)
	}

	cleanup()
	select {
	case <-childReleased:
	case <-time.After(time.Second):
		t.Fatal("child resource release callback was not called")
	}
}

func TestBridgeResolverReconnectsPluginChildAfterResourceClientClose(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	firstHandler := &testReconnectObjectTypeHandler{}
	firstPluginClient := newTestObjectTypePluginClient(t, firstHandler)
	secondHandler := &testReconnectObjectTypeHandler{}
	secondPluginClient := newTestObjectTypePluginClient(t, secondHandler)

	pluginLoader := &testPluginLoadController{client: firstPluginClient}
	rel, err := tb.Bus.AddController(ctx, pluginLoader, nil)
	if err != nil {
		t.Fatalf("AddController: %v", err)
	}
	defer rel()

	registry := NewObjectTypeRegistryResource()
	registry.registrations[1] = &objectTypeRegistration{registration: &s4wave_objecttype_registry.ObjectTypeRegistration{
		TypeId:         "test/type",
		RegistrationId: 1,
		PluginId:       "test-plugin",
	}}
	ctrl := NewBridgeController(le, tb.Bus, registry)
	rel, err = tb.Bus.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatalf("AddController bridge: %v", err)
	}
	defer rel()

	ot, ref, err := objecttype.ExLookupObjectType(ctx, tb.Bus, "test/type")
	if err != nil {
		t.Fatalf("ExLookupObjectType: %v", err)
	}
	if ref != nil {
		defer ref.Release()
	}
	if ot == nil {
		t.Fatalf("expected object type")
	}

	invoker, cleanup, err := ot.GetFactory()(ctx, le, tb.Bus, tb.Engine, nil, "test/object")
	if err != nil {
		t.Fatalf("object type factory: %v", err)
	}
	defer cleanup()
	bridgeInvoker, ok := invoker.(*pluginObjectTypeInvoker)
	if !ok {
		t.Fatalf("object type factory invoker = %T, want *pluginObjectTypeInvoker", invoker)
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))

	if err := client.ExecCall(ctx, "test.Child", "Ping", &testPingMessage{}, &testPingMessage{}); err != nil {
		t.Fatalf("first child ping: %v", err)
	}
	if got := firstHandler.successfulPings(); got != 1 {
		t.Fatalf("first plugin successful pings = %d, want 1", got)
	}
	if got := secondHandler.invokeObjectTypeCalls(); got != 0 {
		t.Fatalf("second plugin InvokeObjectType calls before replacement = %d, want 0", got)
	}

	pluginLoader.SetClient(secondPluginClient)
	bridgeInvoker.session.mtx.Lock()
	firstResourceClient := bridgeInvoker.session.resources
	bridgeInvoker.session.mtx.Unlock()
	firstResourceClient.Release()

	if err := client.ExecCall(ctx, "test.Child", "Ping", &testPingMessage{}, &testPingMessage{}); err != nil {
		t.Fatalf("second child ping after resource client reconnect: %v", err)
	}
	if got := firstHandler.childInvocations(); got != 1 {
		t.Fatalf("first plugin child invocations = %d, want 1", got)
	}
	if got := secondHandler.invokeObjectTypeCalls(); got != 1 {
		t.Fatalf("second plugin InvokeObjectType calls = %d, want 1", got)
	}
	if got := secondHandler.successfulPings(); got != 1 {
		t.Fatalf("second plugin successful pings = %d, want 1", got)
	}
}

func TestBridgeResolverInvokesCallerAttachedHandler(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	registry, resources, registryClient := newRegistryResourceClient(t, ctx)
	defer resources.Release()
	childReleased := make(chan struct{}, 1)
	handlerRoot := srpc.NewMux()
	if err := s4wave_objecttype_registry.SRPCRegisterObjectTypeHandlerService(handlerRoot, &testObjectTypeHandler{childReleased: childReleased}); err != nil {
		t.Fatal(err)
	}
	handlerService := srpc.NewMux()
	if err := resource_server.NewResourceServer(handlerRoot).Register(handlerService); err != nil {
		t.Fatal(err)
	}
	attachedID, err := resources.AttachResource(ctx, "handler", handlerService)
	if err != nil {
		t.Fatalf("attach handler: %v", err)
	}
	registration, err := registryClient.RegisterObjectType(ctx, &s4wave_objecttype_registry.RegisterObjectTypeRequest{
		TypeId:                    "test/type",
		PluginId:                  "test-plugin",
		AttachedHandlerResourceId: attachedID,
	})
	if err != nil {
		t.Fatalf("register attached handler: %v", err)
	}

	ctrl := NewBridgeController(le, tb.Bus, registry)
	rel, err := tb.Bus.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatalf("add bridge: %v", err)
	}
	defer rel()

	ot, ref, err := objecttype.ExLookupObjectType(ctx, tb.Bus, "test/type")
	if err != nil {
		t.Fatalf("lookup attached ObjectType: %v", err)
	}
	defer ref.Release()
	invoker, cleanup, err := ot.GetFactory()(ctx, le, tb.Bus, tb.Engine, nil, "test/object")
	if err != nil {
		t.Fatalf("create attached ObjectType: %v", err)
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))
	if err := client.ExecCall(ctx, "test.Child", "Ping", &testPingMessage{}, &testPingMessage{}); err != nil {
		t.Fatalf("attached child ping: %v", err)
	}
	cleanup()
	select {
	case <-childReleased:
	case <-time.After(time.Second):
		t.Fatal("attached child was not released")
	}

	if err := resources.DetachResource(ctx, attachedID); err != nil {
		t.Fatalf("detach attached handler: %v", err)
	}
	if _, cleanup, err := ot.GetFactory()(ctx, le, tb.Bus, tb.Engine, nil, "test/object"); err == nil {
		if cleanup != nil {
			cleanup()
		}
		t.Fatal("detached attached handler created an ObjectType")
	}
	waitCh := registryChangeWait(registry)
	resources.CreateResourceReference(registration.GetResourceId()).Release()
	select {
	case <-waitCh:
	case <-ctx.Done():
		t.Fatal("registration release did not reach registry")
	}
	if registry.LookupRegistration("test/type") != nil {
		t.Fatal("attached registration remained after release")
	}
}

type testPingMessage struct{}

func (m *testPingMessage) SizeVT() int {
	return 0
}

func (m *testPingMessage) MarshalToSizedBufferVT([]byte) (int, error) {
	return 0, nil
}

func (m *testPingMessage) MarshalVT() ([]byte, error) {
	return []byte{}, nil
}

func (m *testPingMessage) UnmarshalVT([]byte) error {
	return nil
}

func (m *testPingMessage) Reset() {}

type testObjectTypeHandler struct {
	childReleased chan struct{}
}

func (h *testObjectTypeHandler) InvokeObjectType(ctx context.Context, req *s4wave_objecttype_registry.InvokeObjectTypeRequest) (*s4wave_objecttype_registry.InvokeObjectTypeResponse, error) {
	if req.GetTypeId() != "test/type" || req.GetObjectKey() != "test/object" {
		return nil, resource.ErrInvalidResourceID
	}
	if req.GetAttachedEngineResourceId() == 0 {
		return nil, resource.ErrInvalidResourceID
	}
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	engineClient, err := resourceCtx.GetAttachedResource(req.GetAttachedEngineResourceId())
	if err != nil {
		return nil, err
	}
	engine := s4wave_world.NewSRPCEngineResourceServiceClient(engineClient)
	txResp, err := engine.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{Write: true})
	if err != nil {
		return nil, err
	}
	txID := txResp.GetResourceId()
	if txID == 0 {
		return nil, resource.ErrInvalidResourceID
	}
	defer resourceCtx.ReleaseResource(txID)
	txClient, err := resourceCtx.GetAttachedResource(txID)
	if err != nil {
		return nil, err
	}
	worldState := s4wave_world.NewSRPCWorldStateResourceServiceClient(txClient)
	objResp, err := worldState.CreateObject(ctx, &s4wave_world.CreateObjectRequest{
		ObjectKey: "test/objecttype-seed",
	})
	if err != nil {
		return nil, err
	}
	if objID := objResp.GetResourceId(); objID != 0 {
		defer resourceCtx.ReleaseResource(objID)
	}
	tx := s4wave_world.NewSRPCTxResourceServiceClient(txClient)
	if _, err := tx.Commit(ctx, &s4wave_world.CommitRequest{}); err != nil {
		return nil, err
	}
	id, err := resourceCtx.AddResource(srpc.InvokerFunc(func(serviceID string, methodID string, strm srpc.Stream) (bool, error) {
		if serviceID != "test.Child" || methodID != "Ping" {
			return false, nil
		}
		if err := strm.MsgRecv(&testPingMessage{}); err != nil {
			return true, err
		}
		return true, strm.MsgSend(&testPingMessage{})
	}), func() {
		if h.childReleased != nil {
			select {
			case h.childReleased <- struct{}{}:
			default:
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return &s4wave_objecttype_registry.InvokeObjectTypeResponse{ResourceId: id}, nil
}

func newTestObjectTypePluginClient(t *testing.T, handler s4wave_objecttype_registry.SRPCObjectTypeHandlerServiceServer) srpc.Client {
	t.Helper()

	pluginRoot := srpc.NewMux()
	if err := s4wave_objecttype_registry.SRPCRegisterObjectTypeHandlerService(pluginRoot, handler); err != nil {
		t.Fatal(err)
	}
	pluginResourceMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(pluginRoot).Register(pluginResourceMux); err != nil {
		t.Fatal(err)
	}
	return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginResourceMux)))
}

type testReconnectObjectTypeHandler struct {
	mtx                   sync.Mutex
	invokeObjectTypeCount int
	childPingCalls        int
	successfulPingCount   int
}

func (h *testReconnectObjectTypeHandler) InvokeObjectType(ctx context.Context, req *s4wave_objecttype_registry.InvokeObjectTypeRequest) (*s4wave_objecttype_registry.InvokeObjectTypeResponse, error) {
	if req.GetTypeId() != "test/type" || req.GetObjectKey() != "test/object" {
		return nil, resource.ErrInvalidResourceID
	}
	if req.GetAttachedEngineResourceId() == 0 {
		return nil, resource.ErrInvalidResourceID
	}
	h.mtx.Lock()
	h.invokeObjectTypeCount++
	h.mtx.Unlock()

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := resourceCtx.AddResource(srpc.InvokerFunc(h.invokeChild), nil)
	if err != nil {
		return nil, err
	}
	return &s4wave_objecttype_registry.InvokeObjectTypeResponse{ResourceId: id}, nil
}

func (h *testReconnectObjectTypeHandler) invokeChild(serviceID string, methodID string, strm srpc.Stream) (bool, error) {
	if serviceID != "test.Child" || methodID != "Ping" {
		return false, nil
	}

	h.mtx.Lock()
	h.childPingCalls++
	h.mtx.Unlock()

	if err := strm.MsgRecv(&testPingMessage{}); err != nil {
		return true, err
	}
	// Count the ping before responding: the unary caller's ExecCall returns as
	// soon as it receives this response, so an increment after MsgSend races the
	// caller's assertion. Send failure is surfaced through the ExecCall error.
	h.mtx.Lock()
	h.successfulPingCount++
	h.mtx.Unlock()
	if err := strm.MsgSend(&testPingMessage{}); err != nil {
		return true, err
	}
	return true, nil
}

func (h *testReconnectObjectTypeHandler) invokeObjectTypeCalls() int {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.invokeObjectTypeCount
}

func (h *testReconnectObjectTypeHandler) childInvocations() int {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.childPingCalls
}

func (h *testReconnectObjectTypeHandler) successfulPings() int {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	return h.successfulPingCount
}

type testPluginLoadController struct {
	mtx    sync.RWMutex
	client srpc.Client
}

func (c *testPluginLoadController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/plugin-load", controller.MustParseVersion("0.0.1"), "test plugin load")
}

func (c *testPluginLoadController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (c *testPluginLoadController) SetClient(client srpc.Client) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.client = client
}

func (c *testPluginLoadController) Client() srpc.Client {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
	return c.client
}

func (c *testPluginLoadController) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok || dir.LoadPluginID() != "test-plugin" {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]bldr_plugin.LoadPluginValue{bldr_plugin.NewRunningPlugin(c.Client())}), nil)
}

func (c *testPluginLoadController) Close() error {
	return nil
}

type testResourceClientContext struct {
	ctx context.Context
}

func (c *testResourceClientContext) Context() context.Context {
	return c.ctx
}

func (c *testResourceClientContext) AddResource(srpc.Invoker, func()) (uint32, error) {
	return 0, resource.ErrResourceNotFound
}

func (c *testResourceClientContext) AddResourceValue(srpc.Invoker, any, func()) (uint32, error) {
	return 0, resource.ErrResourceNotFound
}

func (c *testResourceClientContext) ReleaseResource(uint32) bool {
	return false
}

func (c *testResourceClientContext) GetResourceValue(uint32) (any, error) {
	return nil, resource.ErrResourceNotFound
}

func (c *testResourceClientContext) GetAttachedResource(uint32) (srpc.Client, error) {
	return nil, resource.ErrResourceNotFound
}

// _ is a type assertion
var _ controller.Controller = (*testPluginLoadController)(nil)

// _ is a type assertion
var _ s4wave_objecttype_registry.SRPCObjectTypeHandlerServiceServer = (*testObjectTypeHandler)(nil)

// _ is a type assertion
var _ resource_server.ResourceClientContext = (*testResourceClientContext)(nil)
