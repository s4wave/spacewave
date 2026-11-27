package plugin_host_resource

import (
	"context"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	"github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	sdk_plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
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
	// Retain a new resource identifier with its mux, value and release callback.
	c.nextID++
	resourceID := c.nextID
	c.muxes[resourceID] = mux
	c.values[resourceID] = value
	c.releases[resourceID] = releaseFn
	return resourceID, nil
}

func (c *testResourceClientContext) ReleaseResource(resourceID uint32) bool {
	// Release the retained resource and remove its mock client records.
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

func TestPluginHostRootReportsInitialCapabilityRegistrationTerminalState(t *testing.T) {
	// Verify completed capability registration reports success when the plugin root is released.
	ctx := t.Context()
	hostRoot := plugin_host_root.NewRoot()
	var completed []bool
	pluginRoot := NewPluginHostRoot(
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

	// Verify releasing an incomplete plugin root reports unsuccessful registration.
	completed = nil
	pluginRoot = NewPluginHostRoot(
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
	// Create a plugin root that accesses the process-lifetime desktop tray.
	ctx := context.Background()
	hostRoot := plugin_host_root.NewRoot()
	pluginRoot := NewPluginHostRoot(nil, "test-plugin", "main", nil, nil, nil, hostRoot, "atoms", "volume", nil)
	pluginClient := newTestResourceClientContext(ctx)

	// Access the desktop tray and verify its plugin resource exposes registration.
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

	// Register a desktop tray entry through the process-lifetime root mux.
	regClient := newTestResourceClientContext(ctx)
	tray := desktop_tray.NewSRPCDesktopTrayResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(hostRoot.GetMux()))))
	regResp, err := tray.RegisterDesktopTrayEntry(
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

	// Verify the tray entry survives release of the accessing plugin root.
	pluginRoot.Release()
	state := snapshotDesktopTray(t, tray)
	if len(state.GetEntries()) != 1 {
		t.Fatalf("expected host root entry to survive plugin root release, got %d", len(state.GetEntries()))
	}

	// Release the tray registration and verify its entry disappears.
	if !regClient.ReleaseResource(regResp.GetResourceId()) {
		t.Fatal("expected registration release")
	}
	state = snapshotDesktopTray(t, tray)
	if len(state.GetEntries()) != 0 {
		t.Fatalf("expected registration release to remove entry, got %d", len(state.GetEntries()))
	}
}

// snapshotDesktopTray captures one tray snapshot through its resource service.
func snapshotDesktopTray(t *testing.T, tray desktop_tray.SRPCDesktopTrayResourceServiceClient) *desktop_tray.DesktopTrayState {
	// Open the tray watch and release it after the first snapshot.
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	strm, err := tray.WatchDesktopTray(ctx, &desktop_tray.WatchDesktopTrayRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer strm.Close()

	// Read the current tray state.
	resp, err := strm.Recv()
	if err != nil {
		t.Fatal(err)
	}
	return resp.GetState()
}

// _ checks the mock resource client context contract.
var _ resource_server.ResourceClientContext = (*testResourceClientContext)(nil)
