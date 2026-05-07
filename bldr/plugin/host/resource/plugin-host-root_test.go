package plugin_host_resource

import (
	"context"
	"io"
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

func TestPluginHostRootAccessDesktopTrayUsesProcessLifetimeRoot(t *testing.T) {
	ctx := context.Background()
	hostRoot := plugin_host_root.NewRoot()
	pluginRoot := NewPluginHostRoot(nil, nil, "test-plugin", "main", nil, nil, nil, hostRoot, "atoms", "volume")
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
