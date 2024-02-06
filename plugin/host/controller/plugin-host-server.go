package plugin_host_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
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
		PluginId: s.pluginID,
		ManifestRef: bldr_manifest.NewManifestRef(
			s.manifest.manifest.GetMeta().CloneVT(),
			s.manifest.manifestRef.Clone(),
		),
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

	dir := plugin.NewLoadPlugin(pluginID)
	resp := ccontainer.NewCContainerVT[*plugin.LoadPluginResponse](nil)

	errCh := make(chan error, 1)
	pushErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	ctx := strm.Context()
	reqCtx, reqCtxCancel := context.WithCancel(ctx)
	defer reqCtxCancel()

	var vals []directive.AttachedValue
	updResp := func() {
		resp.SetValue(&plugin.LoadPluginResponse{
			PluginStatus: &plugin.PluginStatus{
				PluginId: pluginID,
				Running:  len(vals) != 0,
			},
		})
	}

	di, ref, err := s.c.bus.AddDirective(
		dir,
		bus.NewCallbackHandler(
			func(av directive.AttachedValue) {
				vals = append(vals, av)
				if len(vals) == 1 {
					updResp()
				}
			},
			func(av directive.AttachedValue) {
				for i, val := range vals {
					if val == av {
						vals = append(vals[:i], vals[i+1:]...)
						updResp()
						break
					}
				}
			},
			func() {
				reqCtxCancel()
			},
		),
	)
	if err != nil {
		return err
	}
	defer ref.Release()

	defer di.AddIdleCallback(func(errs []error) {
		for _, err := range errs {
			if err != nil && err != context.Canceled {
				pushErr(err)
				return
			}
		}
		updResp()
	})()

	var prevTx *plugin.LoadPluginResponse
	for {
		val, err := resp.WaitValueChange(reqCtx, prevTx, errCh)
		if err != nil {
			return err
		}

		prevTx = val
		if val != nil {
			if err := strm.Send(val); err != nil {
				return err
			}
		}
	}
}

// PluginRpc forwards an RPC call to a remote plugin.
// The plugin will remain loaded as long as the RPC is active.
// Component ID: plugin id
func (s *pluginHostServer) PluginRpc(strm plugin.SRPCPluginHost_PluginRpcStream) error {
	return rpcstream.HandleProxyRpcStream(
		strm,
		func(ctx context.Context, pluginID string) (rpcstream.RpcStreamCaller[plugin.SRPCPlugin_PluginRpcClient], string, func(), error) {
			if pluginID == "" {
				return nil, "", nil, plugin.ErrEmptyPluginID
			}
			if pluginID == s.pluginID {
				return nil, "", nil, errors.Errorf("plugin cannot send rpc to itself: %s", pluginID)
			}
			client, clientRef, err := plugin.ExPluginLoadWaitClient(ctx, s.c.bus, pluginID, nil)
			if err != nil {
				return nil, "", nil, err
			}
			srv := plugin.NewSRPCPluginClient(client)
			return srv.PluginRpc, s.pluginID, clientRef.Release, nil
		},
	)
}

// ExecController executes a config set on the host bus.
func (s *pluginHostServer) ExecController(
	req *controller_exec.ExecControllerRequest,
	strm plugin.SRPCPluginHost_ExecControllerStream,
) error {
	ctx := strm.Context()
	s.c.le.Debugf("plugin %q is applying a configset", s.pluginID)
	return req.Execute(ctx, s.c.bus, true, strm.Send)
}

// _ is a type assertion
var _ plugin.SRPCPluginHostServer = ((*pluginHostServer)(nil))
