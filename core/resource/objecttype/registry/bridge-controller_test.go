package resource_objecttype_registry

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

func TestBridgeResolverKeepsPluginResourceClientAfterRequestContextCancel(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	pluginRoot := srpc.NewMux()
	if err := s4wave_objecttype_registry.SRPCRegisterObjectTypeHandlerService(pluginRoot, &testObjectTypeHandler{}); err != nil {
		t.Fatal(err)
	}
	pluginResourceMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(pluginRoot).Register(pluginResourceMux); err != nil {
		t.Fatal(err)
	}
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginResourceMux)))

	b := inmem.NewBus(cdc.NewController(ctx, le))
	rel, err := b.AddController(ctx, &testPluginLoadController{client: pluginClient}, nil)
	if err != nil {
		t.Fatalf("AddController: %v", err)
	}
	defer rel()
	registry := NewObjectTypeRegistryResource()
	registry.registrations[1] = &s4wave_objecttype_registry.ObjectTypeRegistration{
		TypeId:         "test/type",
		RegistrationId: 1,
		PluginId:       "test-plugin",
	}
	ctrl := NewBridgeController(le, b, registry)
	rel, err = b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatalf("AddController bridge: %v", err)
	}
	defer rel()

	ownerCtx, ownerCancel := context.WithCancel(ctx)
	defer ownerCancel()
	requestCtx, requestCancel := context.WithCancel(resource_server.WithResourceClientContext(ctx, &testResourceClientContext{ctx: ownerCtx}))

	ot, ref, err := objecttype.ExLookupObjectType(ctx, b, "test/type")
	if err != nil {
		t.Fatalf("ExLookupObjectType: %v", err)
	}
	if ref != nil {
		defer ref.Release()
	}
	if !objectTypeRegistryBridgeEnabled() {
		if ot != nil {
			t.Fatalf("expected disabled GoScript bridge to return no object type")
		}
		requestCancel()
		return
	}
	if ot == nil {
		t.Fatalf("expected object type")
	}
	invoker, cleanup, err := ot.GetFactory()(requestCtx, le, b, nil, nil, "test/object")
	if err != nil {
		t.Fatalf("object type factory: %v", err)
	}
	defer cleanup()

	requestCancel()

	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))
	if err := client.ExecCall(ctx, "test.Child", "Ping", &testPingMessage{}, &testPingMessage{}); err != nil {
		t.Fatalf("child resource call after request context cancel: %v", err)
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

type testObjectTypeHandler struct{}

func (h *testObjectTypeHandler) InvokeObjectType(ctx context.Context, _ *s4wave_objecttype_registry.InvokeObjectTypeRequest) (*s4wave_objecttype_registry.InvokeObjectTypeResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
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
	}), nil)
	if err != nil {
		return nil, err
	}
	return &s4wave_objecttype_registry.InvokeObjectTypeResponse{ResourceId: id}, nil
}

type testPluginLoadController struct {
	client srpc.Client
}

func (c *testPluginLoadController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/plugin-load", controller.MustParseVersion("0.0.1"), "test plugin load")
}

func (c *testPluginLoadController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (c *testPluginLoadController) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok || dir.LoadPluginID() != "test-plugin" {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]bldr_plugin.LoadPluginValue{bldr_plugin.NewRunningPlugin(c.client)}), nil)
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
