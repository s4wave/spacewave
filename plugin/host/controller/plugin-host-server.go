package plugin_host_controller

import (
	"context"

	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
)

// pluginHostServer implements the PluginHost
type pluginHostServer struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
	// manifest is the plugin manifest snapshot
	manifest pluginManifestSnapshot
}

// newPluginHostServer constructs a new pluginHostServer.
func newPluginHostServer(c *Controller, pluginID string, manifest pluginManifestSnapshot) *pluginHostServer {
	return &pluginHostServer{
		c:        c,
		pluginID: pluginID,
		manifest: manifest,
	}
}

// GetPluginInfo returns information about the currently running plugin.
func (s *pluginHostServer) GetPluginInfo(
	ctx context.Context,
	req *plugin.GetPluginInfoRequest,
) (*plugin.GetPluginInfoResponse, error) {
	return &plugin.GetPluginInfoResponse{
		PluginId:        s.pluginID,
		PluginManifest:  s.manifest.manifestRef.Clone(),
		VolumeId:        s.c.conf.GetVolumeId(),
		VolumeServiceId: s.c.conf.GetVolumeServiceId(),
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
	var lastResp *plugin.LoadPluginResponse
	s.c.le.Debugf("plugin %q is loading plugin %q via rpc request", s.pluginID, pluginID)
	return plugin_host.ExLoadPlugin(strm.Context(), s.c.bus, pluginID, func(val plugin_host.LoadPluginValue) error {
		resp := &plugin.LoadPluginResponse{
			PluginStatus: &plugin.PluginStatus{
				PluginId: val.PluginId,
				Running:  val.RpcClient != nil,
			},
		}
		if !resp.EqualVT(lastResp) {
			lastResp = resp
			return strm.Send(resp)
		}
		return nil
	})
}

// _ is a type assertion
var _ plugin.SRPCPluginHostServer = ((*pluginHostServer)(nil))
