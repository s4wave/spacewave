package plugin_host

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

// PluginHostServer implements the PluginHost rpc service
type PluginHostServer struct {
	// b is the bus
	b bus.Bus
	// le is the logger
	le *logrus.Entry
	// pluginID is the plugin id.
	pluginID string
	// instanceKey is the plugin instance key.
	instanceKey string
	// manifestSnapshot is the plugin manifestSnapshot snapshot
	manifestSnapshot *bldr_manifest.ManifestSnapshot
	// historical suppresses current-generation registration in an immutable replay worker.
	historical bool
	// prepared delays publication until the scheduler admits this candidate.
	prepared bool
	// registrationInstanceKey identifies the logical installation, independent of RPC routing.
	registrationInstanceKey string
	// hostVolumeInfo is the host volume information
	hostVolumeInfo *volume.VolumeInfo
	// hostStorageID is the Storage ID on the host bus that this plugin
	// instance allocates named volumes through. Empty selects the host
	// default storage.
	hostStorageID string
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
	instanceKey string,
	manifest *bldr_manifest.ManifestSnapshot,
	hostVolumeInfo *volume.VolumeInfo,
	hostStorageID string,
	historical bool,
) *PluginHostServer {
	s := &PluginHostServer{
		b:                       b,
		le:                      le,
		pluginID:                pluginID,
		instanceKey:             instanceKey,
		registrationInstanceKey: instanceKey,
		manifestSnapshot:        manifest,
		hostVolumeInfo:          hostVolumeInfo,
		hostStorageID:           hostStorageID,
		historical:              historical,
	}
	s.pluginFsTracker = keyed.NewKeyedRefCountWithLogger(
		s.newPluginHostServerFsTracker,
		le,
		keyed.WithRetry[string, *pluginHostServerFsTracker](&backoff.Backoff{}),
	)
	s.pluginFsTracker.SetContext(ctx, true)
	return s
}

// SetPrepared requests private registration until explicit candidate admission.
func (s *PluginHostServer) SetPrepared(prepared bool) {
	s.prepared = prepared
}

// SetRegistrationInstanceKey binds registrations to the logical installation.
// Set it before exposing this server to its plugin.
func (s *PluginHostServer) SetRegistrationInstanceKey(instanceKey string) {
	s.registrationInstanceKey = instanceKey
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
		HostStorageId:  s.hostStorageID,
		Historical:     s.historical,
		Prepared:       s.prepared,
		InstanceKey:    s.registrationInstanceKey,
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
	instanceKey, err := s.resolveInstanceKey(req.GetInstanceKey())
	if err != nil {
		return err
	}
	if instanceKey != req.GetInstanceKey() {
		req = req.CloneVT()
		req.InstanceKey = instanceKey
	}
	s.le.Debugf("plugin %q is loading plugin %q via rpc request", s.pluginID, pluginID)
	return HandleLoadPluginRpc(s.b, req, strm)
}

// resolveInstanceKey resolves the effective instance key for a request,
// rejecting requests from a plugin instance that address a foreign
// instance.
func (s *PluginHostServer) resolveInstanceKey(instanceKey string) (string, error) {
	if instanceKey == "" {
		return s.instanceKey, nil
	}
	if s.instanceKey != "" && instanceKey != s.instanceKey {
		return "", errors.Errorf("plugin instance %q cannot access foreign instance %q", s.instanceKey, instanceKey)
	}
	return instanceKey, nil
}

// PluginRpc forwards an RPC call to a remote plugin.
// The plugin will remain loaded as long as the RPC is active.
// Component ID: plugin id, or plugin id / instance key for instanced plugins.
func (s *PluginHostServer) PluginRpc(strm bldr_plugin.SRPCPluginHost_PluginRpcStream) error {
	return rpcstream.HandleProxyRpcStream(
		strm,
		func(ctx context.Context, componentID string) (rpcstream.RpcStreamCaller[bldr_plugin.SRPCPlugin_PluginRpcClient], string, func(), error) {
			pluginID, instanceKey, manifestRoot, err := bldr_plugin.ParsePluginRpcComponentID(componentID)
			if err != nil {
				return nil, "", nil, err
			}
			if pluginID == "" {
				return nil, "", nil, bldr_plugin.ErrEmptyPluginID
			}
			if pluginID == s.pluginID && instanceKey == "" {
				return nil, "", nil, errors.Errorf("plugin cannot send rpc to itself: %s", pluginID)
			}
			instanceKey, err = s.resolveInstanceKey(instanceKey)
			if err != nil {
				return nil, "", nil, err
			}
			dir := bldr_plugin.NewLoadPluginInstanced(pluginID, instanceKey)
			if manifestRoot != "" {
				dir = bldr_plugin.NewLoadPluginAtManifest(pluginID, instanceKey, manifestRoot)
			}
			running, _, clientRef, err := bus.ExecWaitValue[bldr_plugin.RunningPlugin](
				ctx, s.b, dir, bus.ReturnIfIdle(manifestRoot != ""), nil, nil,
			)
			if err != nil {
				return nil, "", nil, err
			}
			if running == nil {
				return nil, "", nil, errors.Errorf("plugin %s: exact manifest %s is unavailable", pluginID, manifestRoot)
			}
			srv := bldr_plugin.NewSRPCPluginClient(running.GetRpcClient())
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

			// Self-access retains this execution's files after a newer revision loads.
			if pluginID == "" || pluginID == s.pluginID {
				pluginID = bldr_plugin.PluginArtifactID(s.pluginID, s.manifestSnapshot.GetManifestRef().GetRootRef().GetHash().MarshalString())
			}

			// wait for reference to be ready
			pluginRef, data, _ := s.pluginFsTracker.AddKeyRef(pluginID)

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
				pluginRef.Release()
				return nil, nil, errors.Errorf("unexpected unixfs id prefix: %v", matchedPrefix)
			}

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
var _ bldr_plugin.SRPCPluginHostServer = (*PluginHostServer)(nil)
