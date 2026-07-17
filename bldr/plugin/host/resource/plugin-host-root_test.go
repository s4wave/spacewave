package plugin_host_resource

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/starpc/srpc"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	sdk_plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
	resource_objecttype_registry "github.com/s4wave/spacewave/core/resource/objecttype/registry"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	"github.com/sirupsen/logrus"
)

type testResourceClientContext struct {
	ctx context.Context

	nextID   uint32
	muxes    map[uint32]srpc.Invoker
	values   map[uint32]any
	releases map[uint32]func()
}

func newTestResourceClientContext(ctx context.Context) *testResourceClientContext {
	return &testResourceClientContext{
		ctx:      ctx,
		muxes:    make(map[uint32]srpc.Invoker),
		values:   make(map[uint32]any),
		releases: make(map[uint32]func()),
	}
}

func (c *testResourceClientContext) Context() context.Context {
	return c.ctx
}

func (c *testResourceClientContext) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

func (c *testResourceClientContext) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	c.nextID++
	resourceID := c.nextID
	c.muxes[resourceID] = mux
	c.values[resourceID] = value
	c.releases[resourceID] = releaseFn
	return resourceID, nil
}

func (c *testResourceClientContext) ReleaseResource(resourceID uint32) bool {
	releaseFn := c.releases[resourceID]
	if releaseFn == nil {
		return false
	}
	delete(c.muxes, resourceID)
	delete(c.values, resourceID)
	delete(c.releases, resourceID)
	releaseFn()
	return true
}

func (c *testResourceClientContext) GetResourceValue(resourceID uint32) (any, error) {
	value, ok := c.values[resourceID]
	if !ok {
		return nil, resource.ErrResourceNotFound
	}
	return value, nil
}

func (c *testResourceClientContext) GetAttachedResource(id uint32) (srpc.Client, error) {
	return nil, resource.ErrResourceNotFound
}

type coreResourceController struct {
	mux srpc.Invoker
}

func (c *coreResourceController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/core-resource", controller.MustParseVersion("0.0.1"), "test core resource")
}

func (c *coreResourceController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (c *coreResourceController) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch dir := inst.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		if dir.LookupRpcServiceID() == resource.SRPCResourceServiceServiceID &&
			dir.LookupRpcServerID() == "" {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c.mux), nil)
		}
	case bldr_plugin.LoadPlugin:
		if dir.LoadPluginID() == "spacewave-core" {
			return nil, errors.New("unexpected core plugin load")
		}
	}
	return nil, nil
}

func (c *coreResourceController) Close() error {
	return nil
}

type testWatchStream struct {
	ctx    context.Context
	cancel func()
	state  *desktop_tray.DesktopTrayState
}

func (s *testWatchStream) Context() context.Context {
	return s.ctx
}

func (s *testWatchStream) MsgSend(msg srpc.Message) error {
	return nil
}

func (s *testWatchStream) MsgRecv(msg srpc.Message) error {
	return io.EOF
}

func (s *testWatchStream) CloseSend() error {
	return nil
}

func (s *testWatchStream) Close() error {
	s.cancel()
	return nil
}

func (s *testWatchStream) Send(resp *desktop_tray.WatchDesktopTrayResponse) error {
	s.state = resp.GetState().CloneVT()
	s.cancel()
	return nil
}

func (s *testWatchStream) SendAndClose(resp *desktop_tray.WatchDesktopTrayResponse) error {
	if resp != nil {
		return s.Send(resp)
	}
	s.cancel()
	return nil
}

func TestPluginHostRootRegistersObjectTypeThroughCore(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	b := inmem.NewBus(directive_controller.NewController(ctx, le))

	registry := resource_objecttype_registry.NewObjectTypeRegistryResource()
	coreMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(registry.GetMux()).Register(coreMux); err != nil {
		t.Fatal(err)
	}
	coreController := &coreResourceController{mux: coreMux}
	releaseCoreController, err := b.AddController(ctx, coreController, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseCoreController)

	hostRoot := plugin_host_root.NewRoot()
	pluginRoot := NewPluginHostRoot(ctx, le, b, "test-plugin", "main", nil, nil, nil, hostRoot, "atoms", "volume", nil)
	pluginClient := newTestResourceClientContext(ctx)
	register := func() (*sdk_plugin_host.RegisterObjectTypeResponse, error) {
		return pluginRoot.RegisterObjectType(
			resource_server.WithResourceClientContext(ctx, pluginClient),
			&sdk_plugin_host.RegisterObjectTypeRequest{
				TypeId: "test/type",
				Metadata: &s4wave_objecttype_registry.ObjectTypeMetadata{
					DisplayName: "Test Type",
				},
			},
		)
	}

	resp, err := register()
	if err != nil {
		t.Fatal(err)
	}
	registration := registry.LookupRegistration("test/type")
	if registration == nil {
		t.Fatal("expected core ObjectType registration")
	}
	if registration.GetPluginId() != "test-plugin" {
		t.Fatalf("plugin ID = %q, want test-plugin", registration.GetPluginId())
	}
	if registration.GetMetadata().GetDisplayName() != "Test Type" {
		t.Fatalf("display name = %q, want Test Type", registration.GetMetadata().GetDisplayName())
	}

	if _, err := register(); err == nil {
		t.Fatal("expected duplicate ObjectType registration to fail")
	}
	if !pluginClient.ReleaseResource(resp.GetResourceId()) {
		t.Fatal("expected registration resource release")
	}
	if registration := registry.LookupRegistration("test/type"); registration != nil {
		t.Fatal("registration remained after resource release")
	}

	if _, err := register(); err != nil {
		t.Fatal(err)
	}
	pluginRoot.Release()
	if registration := registry.LookupRegistration("test/type"); registration != nil {
		t.Fatal("registration remained after plugin root release")
	}
}

func TestPluginHostRootReportsInitialCapabilityRegistrationTerminalState(t *testing.T) {
	ctx := t.Context()
	hostRoot := plugin_host_root.NewRoot()
	var completed []bool
	pluginRoot := NewPluginHostRoot(
		ctx,
		nil,
		nil,
		"test-plugin",
		"main",
		nil,
		nil,
		nil,
		hostRoot,
		"atoms",
		"volume",
		func(complete bool) {
			completed = append(completed, complete)
		},
	)
	if _, err := pluginRoot.CompleteInitialCapabilityRegistration(
		ctx,
		&sdk_plugin_host.CompleteInitialCapabilityRegistrationRequest{},
	); err != nil {
		t.Fatal(err)
	}
	pluginRoot.Release()
	if len(completed) != 1 || !completed[0] {
		t.Fatalf("completion reports = %v, want [true]", completed)
	}

	completed = nil
	pluginRoot = NewPluginHostRoot(
		ctx,
		nil,
		nil,
		"test-plugin",
		"main",
		nil,
		nil,
		nil,
		hostRoot,
		"atoms",
		"volume",
		func(complete bool) {
			completed = append(completed, complete)
		},
	)
	pluginRoot.Release()
	if len(completed) != 1 || completed[0] {
		t.Fatalf("release reports = %v, want [false]", completed)
	}
}

func TestPluginHostRootAccessDesktopTrayUsesProcessLifetimeRoot(t *testing.T) {
	ctx := context.Background()
	hostRoot := plugin_host_root.NewRoot()
	pluginRoot := NewPluginHostRoot(ctx, nil, nil, "test-plugin", "main", nil, nil, nil, hostRoot, "atoms", "volume", nil)
	pluginClient := newTestResourceClientContext(ctx)

	resp, err := pluginRoot.AccessDesktopTray(
		resource_server.WithResourceClientContext(ctx, pluginClient),
		&sdk_plugin_host.AccessDesktopTrayRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	trayMux := pluginClient.muxes[resp.GetResourceId()]
	query, ok := trayMux.(srpc.QueryableInvoker)
	if !ok {
		t.Fatal("expected queryable tray resource mux")
	}
	if !query.HasServiceMethod(
		desktop_tray.SRPCDesktopTrayResourceServiceServiceID,
		"RegisterDesktopTrayEntry",
	) {
		t.Fatal("expected desktop tray service on plugin-accessible resource")
	}

	regClient := newTestResourceClientContext(ctx)
	regResp, err := hostRoot.GetDesktopTray().RegisterDesktopTrayEntry(
		resource_server.WithResourceClientContext(ctx, regClient),
		&desktop_tray.RegisterDesktopTrayEntryRequest{
			Entry: &desktop_tray.DesktopTrayEntry{
				Id:      "status",
				Kind:    desktop_tray.DesktopTrayEntryKind_DESKTOP_TRAY_ENTRY_KIND_STATUS,
				Label:   "Runtime",
				Enabled: true,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	pluginRoot.Release()
	state := snapshotDesktopTray(t, hostRoot.GetDesktopTray())
	if len(state.GetEntries()) != 1 {
		t.Fatalf("expected host root entry to survive plugin root release, got %d", len(state.GetEntries()))
	}

	if !regClient.ReleaseResource(regResp.GetResourceId()) {
		t.Fatal("expected registration release")
	}
	state = snapshotDesktopTray(t, hostRoot.GetDesktopTray())
	if len(state.GetEntries()) != 0 {
		t.Fatalf("expected registration release to remove entry, got %d", len(state.GetEntries()))
	}
}

func snapshotDesktopTray(t *testing.T, tray *desktop_tray.DesktopTray) *desktop_tray.DesktopTrayState {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	strm := &testWatchStream{
		ctx:    ctx,
		cancel: cancel,
	}
	err := tray.WatchDesktopTray(&desktop_tray.WatchDesktopTrayRequest{}, strm)
	if err != context.Canceled {
		t.Fatalf("expected canceled watch after snapshot, got %v", err)
	}
	return strm.state
}

// _ is a type assertion
var (
	_ resource_server.ResourceClientContext                              = ((*testResourceClientContext)(nil))
	_ desktop_tray.SRPCDesktopTrayResourceService_WatchDesktopTrayStream = ((*testWatchStream)(nil))
)
