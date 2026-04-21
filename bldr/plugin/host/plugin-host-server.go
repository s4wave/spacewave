package plugin_host

import (
	"context"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
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
	// pluginFsTracker tracks loaded plugin FSCursor servers
	// TODO: we need a KeyedRefCountValue type which resolves a value with the same logic as refcount/refcount.go
	// TODO: that would be a lot simpler and more robust here
	pluginFsTracker *keyed.KeyedRefCount[string, *pluginHostServerFsTracker]
}

// NewPluginHostServer constructs a new PluginHostServer.
func NewPluginHostServer(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	pluginID string,
	manifest *bldr_manifest.ManifestSnapshot,
	hostVolumeInfo *volume.VolumeInfo,
) *PluginHostServer {
	s := &PluginHostServer{
		b:                b,
		le:               le,
		pluginID:         pluginID,
		manifestSnapshot: manifest,
		hostVolumeInfo:   hostVolumeInfo,
	}
	s.pluginFsTracker = keyed.NewKeyedRefCountWithLogger(
		s.newPluginHostServerFsTracker,
		le,
		keyed.WithRetry[string, *pluginHostServerFsTracker](&backoff.Backoff{}),
	)
	s.pluginFsTracker.SetContext(ctx, true)
	return s
}

// GetPluginInfo returns information about the currently running plugin.
func (s *PluginHostServer) GetPluginInfo(
	ctx context.Context,
	req *bldr_plugin.GetPluginInfoRequest,
) (*bldr_plugin.GetPluginInfoResponse, error) {
	return &bldr_plugin.GetPluginInfoResponse{
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
	req *bldr_plugin.LoadPluginRequest,
	strm bldr_plugin.SRPCPluginHost_LoadPluginStream,
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
func (s *PluginHostServer) PluginRpc(strm bldr_plugin.SRPCPluginHost_PluginRpcStream) error {
	return rpcstream.HandleProxyRpcStream(
		strm,
		func(ctx context.Context, pluginID string) (rpcstream.RpcStreamCaller[bldr_plugin.SRPCPlugin_PluginRpcClient], string, func(), error) {
			if pluginID == "" {
				return nil, "", nil, bldr_plugin.ErrEmptyPluginID
			}
			if pluginID == s.pluginID {
				return nil, "", nil, errors.Errorf("plugin cannot send rpc to itself: %s", pluginID)
			}
			client, clientRef, err := bldr_plugin.ExPluginLoadWaitClient(ctx, s.b, pluginID, nil)
			if err != nil {
				return nil, "", nil, err
			}
			srv := bldr_plugin.NewSRPCPluginClient(client)
			return srv.PluginRpc, s.pluginID, clientRef.Release, nil
		},
	)
}

// PluginFsRpc accesses a FSCursorService to access the plugin assets or dist filesystems.
// The plugin will remain loaded as long as the RPC is active.
// Component ID: plugin-assets or plugin-dist
func (s *PluginHostServer) PluginFsRpc(rpcStream bldr_plugin.SRPCPluginHost_PluginFsRpcStream) error {
	return rpcstream.HandleRpcStream(
		rpcStream,
		func(
			ctx context.Context,
			unixfsID string,
			released func(),
		) (srpc.Invoker, func(), error) {
			if unixfsID == "" {
				return nil, nil, errors.New("component id must be set to filesystem id")
			}

			pluginID, matchedPrefix, err := bldr_plugin.ValidatePluginUnixfsID(unixfsID, true)
			if err != nil {
				return nil, nil, err
			}

			// if id is empty set to ours
			if pluginID == "" {
				pluginID = s.pluginID
			}

			// wait for reference to be ready
			pluginRef, data, _ := s.pluginFsTracker.AddKeyRef(pluginID)

			// TODO: if ExecLoadPlugin returns an error, this might never cancel
			// TODO: if the plugin is unloaded, we need to call released(), but do we do that here?
			// luckily we don't expect that to happen
			res, err := data.resultPromiseCtr.Await(ctx)
			if err != nil {
				pluginRef.Release()
				return nil, nil, err
			}

			var mux srpc.Mux
			switch matchedPrefix {
			case bldr_plugin.PluginDistFsIdPrefix:
				mux = res.distMux
			case bldr_plugin.PluginAssetsFsIdPrefix:
				mux = res.assetsMux
			default:
				return nil, nil, errors.Errorf("unexpected unixfs id prefix: %v", matchedPrefix)
			}

			// wrap with verbose
			// mux = srpc.NewVMux(mux, s.le.WithField("vmux", "plugin-fs-rpc"), true)

			// return release func
			return mux, pluginRef.Release, nil
		},
	)
}

// ExecController executes a config set on the host bus.
func (s *PluginHostServer) ExecController(
	req *controller_exec.ExecControllerRequest,
	strm bldr_plugin.SRPCPluginHost_ExecControllerStream,
) error {
	s.le.Debugf("plugin %q is applying a configset", s.pluginID)
	defer s.le.Debugf("plugin %q exited applying a configset", s.pluginID)

	ctx := strm.Context()
	return req.Execute(ctx, s.b, true, strm.Send)
}

// _ is a type assertion
var _ bldr_plugin.SRPCPluginHostServer = ((*PluginHostServer)(nil))
