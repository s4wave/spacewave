package plugin_host_controller

import (
	"context"

	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/hydra/volume"
)

// pluginHostServer implements the PluginHost
type pluginHostServer struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
	// manifest is the plugin manifest snapshot
	manifest pluginManifestSnapshot
	// hostVolumeInfo is the host volume information
	hostVolumeInfo *volume.VolumeInfo
}

// newPluginHostServer constructs a new pluginHostServer.
func newPluginHostServer(
	c *Controller,
	pluginID string,
	manifest pluginManifestSnapshot,
	hostVolumeInfo *volume.VolumeInfo,
) *pluginHostServer {
	return &pluginHostServer{
		c:              c,
		pluginID:       pluginID,
		manifest:       manifest,
		hostVolumeInfo: hostVolumeInfo,
	}
}

// GetPluginInfo returns information about the currently running plugin.
func (s *pluginHostServer) GetPluginInfo(
	ctx context.Context,
	req *plugin.GetPluginInfoRequest,
) (*plugin.GetPluginInfoResponse, error) {
	return &plugin.GetPluginInfoResponse{
		PluginId:       s.pluginID,
		PluginManifest: s.manifest.manifestRef.Clone(),
		HostVolumeInfo: s.hostVolumeInfo,
	}, nil
}

// LoadPlugin requests to send a LoadPlugin directive.
func (s *pluginHostServer) LoadPlugin(
	req *plugin.LoadPluginRequest,
	strm plugin.SRPCPluginHost_LoadPluginStream,
) error {
	if err := req.Validate(); err != nil {
		return err
	}

	pluginID := req.GetPluginId()
	s.c.le.Debugf("plugin %q is loading plugin %q via rpc request", s.pluginID, pluginID)

	ctx := strm.Context()
ValLoop:
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}

		valCtx, valCtxCancel := context.WithCancel(ctx)
		rp, rpRef, err := plugin_host.ExLoadPlugin(strm.Context(), s.c.bus, false, pluginID, valCtxCancel)
		if err != nil {
			return err
		}

		clientCtr := rp.GetRpcClientCtr()
		val := clientCtr.GetValue()
		var lastResp *plugin.LoadPluginResponse
		for {
			isRunning := val != nil
			resp := &plugin.LoadPluginResponse{
				PluginStatus: &plugin.PluginStatus{
					PluginId: pluginID,
					Running:  isRunning,
				},
			}
			if !resp.EqualVT(lastResp) {
				lastResp = resp
				if err := strm.Send(resp); err != nil {
					rpRef.Release()
					return err
				}
			}
			val, err = clientCtr.WaitValueChange(valCtx, val, nil)
			if err != nil {
				rpRef.Release()
				valCtxCancel()
				continue ValLoop
			}
		}
	}
}

// _ is a type assertion
var _ plugin.SRPCPluginHostServer = ((*pluginHostServer)(nil))
