//go:build !js

package resource_testbed

import (
	"context"

	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	browser_testbed "github.com/s4wave/spacewave/testbed/browser"
	"github.com/sirupsen/logrus"
)

// BrowserTestServer provides a WebSocket-based RPC server for browser E2E tests.
// It exposes a LayoutHost service that can be connected to from browser tests.
//
// Deprecated: Use browser_testbed.LayoutServer directly instead.
type BrowserTestServer = browser_testbed.LayoutServer

// NewBrowserTestServer creates a new BrowserTestServer.
//
// Deprecated: Use browser_testbed.NewLayoutServer directly instead.
func NewBrowserTestServer(le *logrus.Entry, _ any) *BrowserTestServer {
	return browser_testbed.NewLayoutServer(le)
}

// LayoutServerHelper provides helper methods for working with LayoutServer in tests.
type LayoutServerHelper struct {
	*browser_testbed.LayoutServer
}

// NewLayoutServerHelper creates a new LayoutServerHelper wrapping a LayoutServer.
func NewLayoutServerHelper(server *browser_testbed.LayoutServer) *LayoutServerHelper {
	return &LayoutServerHelper{LayoutServer: server}
}

// SetupInitialLayoutModel sets up a standard initial layout model for tests.
func (h *LayoutServerHelper) SetupInitialLayoutModel() {
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
	h.SetLayoutModel(initialModel)
}

// WaitForLayoutUpdateWithTimeout waits for a layout update with context timeout handling.
func (h *LayoutServerHelper) WaitForLayoutUpdateWithTimeout(ctx context.Context) (*s4wave_layout.LayoutModel, error) {
	return h.LayoutServer.WaitForLayoutUpdate(ctx)
}
