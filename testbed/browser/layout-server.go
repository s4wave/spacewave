//go:build !js

package browser_testbed

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	resource_layout "github.com/s4wave/spacewave/core/resource/layout"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	"github.com/sirupsen/logrus"
)

// LayoutServer serves the browser layout test RPC surface over WebSocket.
type LayoutServer struct {
	server         *Server
	le             *logrus.Entry
	layoutStateCtr *ccontainer.CContainer[*s4wave_layout.LayoutModel]

	// bcast protects the pending frontend event fields.
	bcast            broadcast.Broadcast
	lastLayoutUpdate *s4wave_layout.LayoutModel
	lastNavigateTab  *s4wave_layout.NavigateTabRequest
}

// NewLayoutServer creates a new LayoutServer.
func NewLayoutServer(le *logrus.Entry) *LayoutServer {
	layoutStateCtr := ccontainer.NewCContainer[*s4wave_layout.LayoutModel](nil)

	s := &LayoutServer{
		le:             le,
		layoutStateCtr: layoutStateCtr,
	}

	mux := srpc.NewMux()

	layoutResource := resource_layout.NewLayoutResource(
		layoutStateCtr,
		s.handleSetLayout,
		s.handleNavigateTab,
	)
	_ = s4wave_layout.SRPCRegisterLayoutHost(mux, layoutResource)

	s.server = NewServer(le, mux)

	return s
}

// Start starts the WebSocket server on an available loopback port.
func (s *LayoutServer) Start(ctx context.Context) (int, error) {
	return s.server.Start(ctx)
}

// Stop stops the server.
func (s *LayoutServer) Stop(ctx context.Context) error {
	return s.server.Stop(ctx)
}

// SetLayoutModel publishes the server-side layout model to connected clients.
func (s *LayoutServer) SetLayoutModel(model *s4wave_layout.LayoutModel) {
	s.layoutStateCtr.SetValue(model)
}

// GetLayoutModel returns the current layout model.
func (s *LayoutServer) GetLayoutModel() *s4wave_layout.LayoutModel {
	return s.layoutStateCtr.GetValue()
}

// WaitForLayoutUpdate waits for the next frontend layout update.
func (s *LayoutServer) WaitForLayoutUpdate(ctx context.Context) (*s4wave_layout.LayoutModel, error) {
	var result *s4wave_layout.LayoutModel
	err := s.bcast.Wait(ctx, func(broadcast func(), getWaitCh func() <-chan struct{}) (bool, error) {
		if s.lastLayoutUpdate != nil {
			result = s.lastLayoutUpdate
			s.lastLayoutUpdate = nil
			return true, nil
		}
		return false, nil
	})
	return result, err
}

// WaitForNavigateTab waits for the next frontend tab navigation request.
func (s *LayoutServer) WaitForNavigateTab(ctx context.Context) (*s4wave_layout.NavigateTabRequest, error) {
	var result *s4wave_layout.NavigateTabRequest
	err := s.bcast.Wait(ctx, func(broadcast func(), getWaitCh func() <-chan struct{}) (bool, error) {
		if s.lastNavigateTab != nil {
			result = s.lastNavigateTab
			s.lastNavigateTab = nil
			return true, nil
		}
		return false, nil
	})
	return result, err
}

// DrainLayoutUpdates drains any pending layout updates.
func (s *LayoutServer) DrainLayoutUpdates() {
	s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.lastLayoutUpdate = nil
	})
}

// handleSetLayout is called when the frontend pushes a layout update.
func (s *LayoutServer) handleSetLayout(ctx context.Context, layoutModel *s4wave_layout.LayoutModel) error {
	s.le.Debug("received layout update from frontend")
	s.layoutStateCtr.SetValue(layoutModel)
	s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.lastLayoutUpdate = layoutModel
		broadcast()
	})
	return nil
}

// handleNavigateTab is called when the frontend navigates within a tab.
func (s *LayoutServer) handleNavigateTab(ctx context.Context, req *s4wave_layout.NavigateTabRequest) (*s4wave_layout.NavigateTabResponse, error) {
	s.le.Debugf("navigate tab %s to %s", req.GetTabId(), req.GetPath())
	s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		s.lastNavigateTab = req
		broadcast()
	})
	return &s4wave_layout.NavigateTabResponse{}, nil
}

// GetPort returns the port the server is listening on, or 0 if not running.
func (s *LayoutServer) GetPort() int {
	return s.server.GetPort()
}
