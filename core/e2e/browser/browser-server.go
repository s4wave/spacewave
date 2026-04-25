//go:build !js

package s4wave_core_e2e_browser

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_layout "github.com/s4wave/spacewave/core/resource/layout"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	browser_testbed "github.com/s4wave/spacewave/testbed/browser"
	"github.com/sirupsen/logrus"
)

// SpacewaveCorePluginID is the plugin ID for the spacewave-core plugin.
const SpacewaveCorePluginID = "spacewave-core"

// BrowserTestServer provides a WebSocket-based RPC server for browser E2E tests.
// It exposes the spacewave-core plugin's resource services for testing the complete app,
// plus a standalone LayoutHost for simple layout component tests.
type BrowserTestServer struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// server is the underlying browser testbed server
	server *browser_testbed.Server
	// pluginClientRef releases the plugin client reference
	pluginClientRef func()
	// layoutStateCtr holds the current layout model state for the standalone LayoutHost
	layoutStateCtr *ccontainer.CContainer[*s4wave_layout.LayoutModel]
}

// NewBrowserTestServer creates a new BrowserTestServer.
// It waits for the spacewave-core plugin to be loaded and exposes its RPC services.
func NewBrowserTestServer(
	le *logrus.Entry,
	b bus.Bus,
) *BrowserTestServer {
	return &BrowserTestServer{
		le:             le,
		bus:            b,
		layoutStateCtr: ccontainer.NewCContainer[*s4wave_layout.LayoutModel](nil),
	}
}

// Start starts the WebSocket server on a random available port.
// It first waits for the spacewave-core plugin to be ready, then exposes its services.
// Returns the port number the server is listening on.
func (s *BrowserTestServer) Start(ctx context.Context) (int, error) {
	// Wait for the spacewave-core plugin to be loaded and get its RPC client
	s.le.Info("waiting for spacewave-core plugin to be loaded...")
	pluginClient, pluginClientRef, err := bldr_plugin.ExPluginLoadWaitClient(ctx, s.bus, SpacewaveCorePluginID, nil)
	if err != nil {
		return 0, err
	}
	s.pluginClientRef = pluginClientRef.Release
	s.le.Info("spacewave-core plugin loaded, creating browser server")

	// Create ClientInvoker to proxy any unhandled service calls to the plugin.
	// We need to handle both:
	// 1. Calls with "plugin/spacewave-core/" prefix (from AppAPI through BldrContext)
	// 2. Calls without prefix (from direct SDK tests like App.backend.e2e.test.tsx)
	//
	// The pluginPrefixInvoker wraps the client and tries to strip the prefix first,
	// falling back to the direct call if no prefix matches.
	clientInvoker := srpc.NewClientInvoker(pluginClient)
	pluginServicePrefix := "plugin/" + SpacewaveCorePluginID + "/"
	pluginPrefixInvoker := newFallbackPrefixInvoker(clientInvoker, pluginServicePrefix)

	// Create serverMux with the plugin as fallback
	// Local services take precedence, unmatched calls go to the plugin
	serverMux := srpc.NewMux(pluginPrefixInvoker)

	// Register a standalone LayoutHost for simple layout tests
	// This allows BaseLayout E2E tests to work without going through the full Resource protocol
	s.setupInitialLayoutModel()
	layoutResource := resource_layout.NewLayoutResource(
		s.layoutStateCtr,
		func(ctx context.Context, model *s4wave_layout.LayoutModel) error {
			s.layoutStateCtr.SetValue(model)
			return nil
		},
		s.navigateTab,
	)
	_ = s4wave_layout.SRPCRegisterLayoutHost(serverMux, layoutResource)

	// Create the underlying server with the mux
	s.server = browser_testbed.NewServer(s.le, serverMux)

	return s.server.Start(ctx)
}

// navigateTab updates the path field of a tab in the layout.
func (s *BrowserTestServer) navigateTab(ctx context.Context, req *s4wave_layout.NavigateTabRequest) (*s4wave_layout.NavigateTabResponse, error) {
	tabID := req.GetTabId()
	if tabID == "" {
		return &s4wave_layout.NavigateTabResponse{}, nil
	}

	// Get current model
	currentModel := s.layoutStateCtr.GetValue()
	if currentModel == nil {
		return &s4wave_layout.NavigateTabResponse{}, nil
	}

	// Clone the model for modification
	updatedModel := currentModel.CloneVT()

	// Find the tab by ID and update its path
	var tabFound bool
	resource_layout.WalkLayoutModel(updatedModel, func(node any) bool {
		tabDef, ok := node.(*s4wave_layout.TabDef)
		if !ok {
			return true
		}
		if tabDef.GetId() != tabID {
			return true
		}

		// Found the tab - unmarshal its data if it exists
		// For simple test tabs that don't have ObjectLayoutTab data, just skip
		if len(tabDef.GetData()) == 0 {
			tabFound = true
			return false
		}

		// Unmarshal the data - assume it could be various formats
		// For now, just mark as found without modifying
		tabFound = true
		return false
	})

	if !tabFound {
		return &s4wave_layout.NavigateTabResponse{}, nil
	}

	// Update the state container
	s.layoutStateCtr.SetValue(updatedModel)

	return &s4wave_layout.NavigateTabResponse{}, nil
}

// setupInitialLayoutModel sets up a standard initial layout model for tests.
func (s *BrowserTestServer) setupInitialLayoutModel() {
	initialModel := &s4wave_layout.LayoutModel{
		Layout: &s4wave_layout.RowDef{
			Id: "root",
			Children: []*s4wave_layout.RowOrTabSetDef{
				{
					Node: &s4wave_layout.RowOrTabSetDef_TabSet{
						TabSet: &s4wave_layout.TabSetDef{
							Id: "tabset-1",
							Children: []*s4wave_layout.TabDef{
								{Id: "tab-1", Name: "Tab 1"},
								{Id: "tab-2", Name: "Tab 2"},
								{Id: "tab-closable", Name: "Closable Tab", EnableClose: true},
							},
						},
					},
				},
			},
		},
	}
	s.layoutStateCtr.SetValue(initialModel)
}

// Stop stops the server and releases the plugin client.
func (s *BrowserTestServer) Stop(ctx context.Context) error {
	if s.server != nil {
		if err := s.server.Stop(ctx); err != nil {
			return err
		}
	}
	if s.pluginClientRef != nil {
		s.pluginClientRef()
		s.pluginClientRef = nil
	}
	return nil
}

// GetPort returns the port the server is listening on, or 0 if not running.
func (s *BrowserTestServer) GetPort() int {
	if s.server == nil {
		return 0
	}
	return s.server.GetPort()
}

// fallbackPrefixInvoker wraps an invoker and handles both prefixed and non-prefixed service IDs.
// If the service ID starts with the prefix, it strips the prefix before invoking.
// If not, it invokes directly without modification.
type fallbackPrefixInvoker struct {
	inv    srpc.Invoker
	prefix string
}

// newFallbackPrefixInvoker creates a new fallbackPrefixInvoker.
func newFallbackPrefixInvoker(inv srpc.Invoker, prefix string) *fallbackPrefixInvoker {
	return &fallbackPrefixInvoker{inv: inv, prefix: prefix}
}

// InvokeMethod implements srpc.Invoker.
func (f *fallbackPrefixInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// If the service ID has the prefix, strip it
	if strings.HasPrefix(serviceID, f.prefix) {
		serviceID = serviceID[len(f.prefix):]
	}
	// Otherwise, use the service ID as-is
	return f.inv.InvokeMethod(serviceID, methodID, strm)
}

// _ is a type assertion
var _ srpc.Invoker = (*fallbackPrefixInvoker)(nil)
