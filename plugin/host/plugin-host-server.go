package plugin_host

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// PluginHostServer implements the PluginHost rpc service
type PluginHostServer struct {
	// b is the bus
	b bus.Bus
	// le is the logger
	le *logrus.Entry
	// pluginID is the plugin id
	pluginID string
	// manifestSnapshot is the plugin manifestSnapshot snapshot
	manifestSnapshot *bldr_manifest.ManifestSnapshot
	// hostVolumeInfo is the host volume information
	hostVolumeInfo *volume.VolumeInfo
}

// NewPluginHostServer constructs a new PluginHostServer.
func NewPluginHostServer(
	b bus.Bus,
	le *logrus.Entry,
	pluginID string,
	manifest *bldr_manifest.ManifestSnapshot,
	hostVolumeInfo *volume.VolumeInfo,
) *PluginHostServer {
	return &PluginHostServer{
		b:                b,
		le:               le,
		pluginID:         pluginID,
		manifestSnapshot: manifest,
		hostVolumeInfo:   hostVolumeInfo,
	}
}

// GetPluginInfo returns information about the currently running plugin.
func (s *PluginHostServer) GetPluginInfo(
	ctx context.Context,
	req *plugin.GetPluginInfoRequest,
) (*plugin.GetPluginInfoResponse, error) {
	return &plugin.GetPluginInfoResponse{
		PluginId: s.pluginID,
		ManifestRef: bldr_manifest.NewManifestRef(
			s.manifestSnapshot.GetManifest().GetMeta().CloneVT(),
			s.manifestSnapshot.GetManifestRef().Clone(),
		),
		HostVolumeInfo: s.hostVolumeInfo,
	}, nil
}

// LoadPlugin requests to send a LoadPlugin directive.
func (s *PluginHostServer) LoadPlugin(
	req *plugin.LoadPluginRequest,
	strm plugin.SRPCPluginHost_LoadPluginStream,
) error {
	if err := req.Validate(); err != nil {
		return err
	}

	pluginID := req.GetPluginId()
	s.le.Debugf("plugin %q is loading plugin %q via rpc request", s.pluginID, pluginID)

	return HandleLoadPluginRpc(s.b, req, strm)
}

// PluginRpc forwards an RPC call to a remote plugin.
// The plugin will remain loaded as long as the RPC is active.
// Component ID: plugin id
func (s *PluginHostServer) PluginRpc(strm plugin.SRPCPluginHost_PluginRpcStream) error {
	return rpcstream.HandleProxyRpcStream(
		strm,
		func(ctx context.Context, pluginID string) (rpcstream.RpcStreamCaller[plugin.SRPCPlugin_PluginRpcClient], string, func(), error) {
			if pluginID == "" {
				return nil, "", nil, plugin.ErrEmptyPluginID
			}
			if pluginID == s.pluginID {
				return nil, "", nil, errors.Errorf("plugin cannot send rpc to itself: %s", pluginID)
			}
			client, clientRef, err := plugin.ExPluginLoadWaitClient(ctx, s.b, pluginID, nil)
			if err != nil {
				return nil, "", nil, err
			}
			srv := plugin.NewSRPCPluginClient(client)
			return srv.PluginRpc, s.pluginID, clientRef.Release, nil
		},
	)
}

// ExecController executes a config set on the host bus.
func (s *PluginHostServer) ExecController(
	req *controller_exec.ExecControllerRequest,
	strm plugin.SRPCPluginHost_ExecControllerStream,
) error {
	ctx := strm.Context()
	s.le.Debugf("plugin %q is applying a configset", s.pluginID)
	defer s.le.Debugf("plugin %q exited applying a configset", s.pluginID)
	return req.Execute(ctx, s.b, true, strm.Send)
}

// _ is a type assertion
var _ plugin.SRPCPluginHostServer = ((*PluginHostServer)(nil))
