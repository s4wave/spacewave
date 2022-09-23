package plugin_host_controller

import (
	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
)

// pluginHostServer implements the PluginHost
type pluginHostServer struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
}

// newPluginHostServer constructs a new pluginHostServer.
func newPluginHostServer(c *Controller, pluginID string) *pluginHostServer {
	return &pluginHostServer{
		c:        c,
		pluginID: pluginID,
	}
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
	s.c.le.Debugf("plugin %s is loading plugin %s via rpc request", s.pluginID, pluginID)
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
