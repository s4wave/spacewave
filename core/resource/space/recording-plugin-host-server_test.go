package resource_space

import (
	"context"
	"sync"

	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// recordingPluginHostServer is a parent plugin host that records LoadPlugin
// calls and holds each load for its stream lifetime.
type recordingPluginHostServer struct {
	// requests receives each LoadPlugin request.
	requests chan *bldr_plugin.LoadPluginRequest
	// released closes when a LoadPlugin stream ends.
	released chan struct{}
	// once closes released.
	once sync.Once
}

// newRecordingPluginHostServer constructs a recording plugin host.
func newRecordingPluginHostServer() *recordingPluginHostServer {
	return &recordingPluginHostServer{
		requests: make(chan *bldr_plugin.LoadPluginRequest, 1),
		released: make(chan struct{}),
	}
}

// GetPluginInfo returns empty plugin metadata.
func (h *recordingPluginHostServer) GetPluginInfo(
	context.Context,
	*bldr_plugin.GetPluginInfoRequest,
) (*bldr_plugin.GetPluginInfoResponse, error) {
	return &bldr_plugin.GetPluginInfoResponse{}, nil
}

// ExecController rejects the unused controller execution path.
func (h *recordingPluginHostServer) ExecController(
	*controller_exec.ExecControllerRequest,
	bldr_plugin.SRPCPluginHost_ExecControllerStream,
) error {
	return errors.New("ExecController is not used by this test")
}

// LoadPlugin records one plugin load and holds it for its stream lifetime.
func (h *recordingPluginHostServer) LoadPlugin(
	req *bldr_plugin.LoadPluginRequest,
	strm bldr_plugin.SRPCPluginHost_LoadPluginStream,
) error {
	h.requests <- req.CloneVT()
	if err := strm.Send(&bldr_plugin.LoadPluginResponse{
		PluginStatus: &bldr_plugin.PluginStatus{Running: true},
	}); err != nil {
		return err
	}
	<-strm.Context().Done()
	h.once.Do(func() { close(h.released) })
	return strm.Context().Err()
}

// PluginRpc rejects the unused plugin RPC path.
func (h *recordingPluginHostServer) PluginRpc(bldr_plugin.SRPCPluginHost_PluginRpcStream) error {
	return errors.New("PluginRpc is not used by this test")
}

// PluginFsRpc rejects the unused plugin filesystem RPC path.
func (h *recordingPluginHostServer) PluginFsRpc(bldr_plugin.SRPCPluginHost_PluginFsRpcStream) error {
	return errors.New("PluginFsRpc is not used by this test")
}

// WatchPluginStatus rejects the unused status path.
func (h *recordingPluginHostServer) WatchPluginStatus(
	*bldr_plugin.WatchPluginStatusRequest,
	bldr_plugin.SRPCPluginHost_WatchPluginStatusStream,
) error {
	return errors.New("WatchPluginStatus is not used by this test")
}

// _ is a type assertion
var _ bldr_plugin.SRPCPluginHostServer = (*recordingPluginHostServer)(nil)
