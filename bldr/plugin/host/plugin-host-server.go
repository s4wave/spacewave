package plugin_host

import (
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	bldr_frontend "github.com/s4wave/spacewave/bldr/frontend"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/volume"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// PluginHostServer implements the PluginHost rpc service.
type PluginHostServer struct {
	// b is the bus.
	b bus.Bus
	// le is the logger.
	le *logrus.Entry
	// pluginID is the plugin id.
	pluginID string
	// instanceKey is the plugin instance key.
	instanceKey string
	// manifestSnapshot is the plugin manifestSnapshot snapshot.
	manifestSnapshot *bldr_manifest.ManifestSnapshot
	// historical suppresses current-generation registration in an immutable replay worker.
	historical bool
	// prepared delays publication until the scheduler admits this candidate.
	prepared bool
	// namesPeers permits LoadPlugin requests that select the peer whose
	// approvals decide their roots.
	namesPeers bool
	// registrationInstanceKey identifies the logical installation, independent of RPC routing.
	registrationInstanceKey string
	// hostVolumeInfo is the host volume information.
	hostVolumeInfo *volume.VolumeInfo
	// hostStorageID is the Storage ID on the host bus that this plugin
	// instance allocates named volumes through. Empty selects the host
	// default storage.
	hostStorageID string
	// hostExportPluginIDs are the scheduler's trusted plugins: they may
	// select any instance, and the others reach them on the scheduler's
	// key.
	hostExportPluginIDs []string
	// scheduler is the host scheduler running this plugin, if any.
	scheduler bldr_plugin.PluginScheduler
	// pluginFsTracker tracks loaded plugin FSCursor servers.
	// TODO: pluginFsTracker needs a KeyedRefCountValue type that resolves a
	// value with the same logic as refcount/refcount.go.
	// TODO: KeyedRefCountValue would be a lot simpler and less prone to
	// errors here.
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

// SetNamesPeers permits this plugin's LoadPlugin requests to select the peer
// whose approvals decide their roots. Grant this permission only to plugins
// trusted to run plugins on the host, such as one that forwards a session's
// Space loads. A request from any other plugin that selects a peer is refused,
// so a plugin cannot borrow another peer's approvals. Set namesPeers before
// exposing this server to its plugin.
func (s *PluginHostServer) SetNamesPeers(namesPeers bool) {
	s.namesPeers = namesPeers
}

// SetRegistrationInstanceKey binds registrations to the logical installation.
// Set it before exposing this server to its plugin.
func (s *PluginHostServer) SetRegistrationInstanceKey(instanceKey string) {
	s.registrationInstanceKey = instanceKey
}

// SetHostExportPluginIDs sets the scheduler's trusted plugins (see
// loadInstanceKey). Set it before exposing this server to its plugin.
func (s *PluginHostServer) SetHostExportPluginIDs(pluginIDs []string) {
	s.hostExportPluginIDs = pluginIDs
}

// SetPluginScheduler sets the host scheduler whose status WatchPluginStatus
// streams. Set it before exposing this server to its plugin.
func (s *PluginHostServer) SetPluginScheduler(scheduler bldr_plugin.PluginScheduler) {
	s.scheduler = scheduler
}

// GetPluginInfo returns information about the running plugin.
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
	// Validate the request.
	if err := req.Validate(); err != nil {
		return err
	}
	if err := s.refuseLoad(req); err != nil {
		return err
	}

	// Resolve the instance key the caller may load into the request.
	pluginID := req.GetPluginId()
	selectsBuild := len(req.GetManifests()) != 0 || req.GetManifestRoot() != ""
	instanceKey, err := s.loadInstanceKey(pluginID, req.GetInstanceKey(), selectsBuild)
	if err != nil {
		return err
	}
	if instanceKey != req.GetInstanceKey() {
		req = req.CloneVT()
		req.InstanceKey = instanceKey
	}

	// Handle the load request through the bus.
	s.le.Debugf("plugin %q is loading plugin %q via rpc request", s.pluginID, pluginID)
	return HandleLoadPluginRpc(s.b, req, strm)
}

// refuseLoad returns why this server's plugin may not send req, or nil. The
// function enforces every rule about what a request may select beyond the
// caller's own scope: a peer whose approvals decide the roots is selected only
// by a plugin granted SetNamesPeers.
func (s *PluginHostServer) refuseLoad(req *bldr_plugin.LoadPluginRequest) error {
	if req.GetPeerId() != "" && !s.namesPeers {
		return errors.Errorf("plugin %q may not name the approving peer of a load", s.pluginID)
	}
	return nil
}

// loadInstanceKey returns the instance key a load of pluginID by this server's
// plugin runs under, given the key the request selects, which may be empty.
//
// Invariant: a plugin cannot obtain an instance, of itself or of another
// plugin, on a wider volume view than its own. The rules:
//
//   - A host-export caller may select any key. An empty key selects the
//     scheduler's key.
//   - Any other load inherits the caller's own binding key: an empty key or
//     the caller's own key selects that binding key, and any other key is
//     refused.
//
// A load of a host-export plugin under any key runs the scheduler's own
// instance of that plugin. The scheduler makes that decision for every source
// of loads.
//
// Invariant: only the host's own start path pins a build of a host-export
// plugin. A load that selects a build, by manifests or a manifest root, of a
// host-export plugin is refused whatever the key and whoever asks, so no
// plugin starts a second build of one with the host's powers on this volume.
func (s *PluginHostServer) loadInstanceKey(pluginID, instanceKey string, selectsBuild bool) (string, error) {
	switch {
	case selectsBuild && slices.Contains(s.hostExportPluginIDs, pluginID):
		return "", errors.Errorf("plugin %s may not select a build of host plugin %s", s.pluginID, pluginID)
	case s.callerIsHostPlugin():
		if instanceKey == "" {
			return s.instanceKey, nil
		}
		return instanceKey, nil
	case instanceKey == "" || instanceKey == s.registrationInstanceKey:
		return s.registrationInstanceKey, nil
	}
	return "", errors.Errorf("plugin %s may not load %s at instance %q", s.pluginID, pluginID, instanceKey)
}

// PluginRpc forwards an RPC call to a remote plugin.
// The plugin will remain loaded as long as the RPC is active.
// Component ID: plugin id, or plugin id / instance key for instanced plugins.
func (s *PluginHostServer) PluginRpc(strm bldr_plugin.SRPCPluginHost_PluginRpcStream) error {
	return rpcstream.HandleProxyRpcStream(
		strm,
		func(ctx context.Context, componentID string) (rpcstream.RpcStreamCaller[bldr_plugin.SRPCPlugin_PluginRpcClient], string, func(), error) {
			// Parse the component id.
			pluginID, instanceKey, manifestRoot, err := bldr_plugin.ParsePluginRpcComponentID(componentID)
			if err != nil {
				return nil, "", nil, err
			}

			// Open the target's PluginRpc stamped with this plugin's identity.
			caller, release, err := s.openPluginRpc(ctx, pluginID, instanceKey, manifestRoot)
			if err != nil {
				return nil, "", nil, err
			}
			return caller, s.caller().ComponentID(), release, nil
		},
	)
}

// openPluginRpc loads pluginID at the instance this server's plugin may reach
// and returns the target's PluginRpc caller and a release function. The caller
// opens the stream with this server's PluginCaller as the component ID, so the
// target serves the call under "plugin/<caller>" and never on its root mux.
func (s *PluginHostServer) openPluginRpc(
	ctx context.Context,
	pluginID, instanceKey, manifestRoot string,
) (rpcstream.RpcStreamCaller[bldr_plugin.SRPCPlugin_PluginRpcClient], func(), error) {
	// Reject an empty target and self-calls.
	if pluginID == "" {
		return nil, nil, bldr_plugin.ErrEmptyPluginID
	}
	if pluginID == s.pluginID && instanceKey == "" {
		return nil, nil, errors.Errorf("plugin cannot send rpc to itself: %s", pluginID)
	}

	// Build the load directive for the target plugin.
	instanceKey, err := s.loadInstanceKey(pluginID, instanceKey, manifestRoot != "")
	if err != nil {
		return nil, nil, err
	}
	dir := bldr_plugin.NewLoadPluginInstanced(pluginID, instanceKey)
	if manifestRoot != "" {
		dir = bldr_plugin.NewLoadPluginAtManifest(pluginID, instanceKey, manifestRoot)
	}

	// Wait for the plugin to run and return its PluginRpc caller.
	running, _, clientRef, err := bus.ExecWaitValue[bldr_plugin.RunningPlugin](
		ctx, s.b, dir, bus.ReturnIfIdle(manifestRoot != ""), nil, nil,
	)
	if err != nil {
		return nil, nil, err
	}
	if running == nil {
		return nil, nil, errors.Errorf("plugin %s: exact manifest %s is unavailable", pluginID, manifestRoot)
	}
	srv := bldr_plugin.NewSRPCPluginClient(running.GetRpcClient())
	return srv.PluginRpc, clientRef.Release, nil
}

// PluginFsRpc accesses a FSCursorService to access the plugin assets or dist filesystems.
// The plugin will remain loaded as long as the RPC is active.
// Component ID: plugin-assets or plugin-dist.
func (s *PluginHostServer) PluginFsRpc(rpcStream bldr_plugin.SRPCPluginHost_PluginFsRpcStream) error {
	return rpcstream.HandleRpcStream(
		rpcStream,
		func(
			ctx context.Context,
			unixfsID string,
			released func(),
		) (srpc.Invoker, func(), error) {
			// The component id must identify a filesystem.
			if unixfsID == "" {
				return nil, nil, errors.New("component id must be set to filesystem id")
			}

			// Validate the unixfs id and resolve the owning plugin.
			pluginID, matchedPrefix, err := bldr_plugin.ValidatePluginUnixfsID(unixfsID, true)
			if err != nil {
				return nil, nil, err
			}

			// Self-access retains this execution's files after a newer revision loads.
			if pluginID == "" || pluginID == s.pluginID {
				pluginID = bldr_plugin.PluginArtifactID(s.pluginID, s.manifestSnapshot.GetManifestRef().GetRootRef().GetHash().MarshalString())
			}

			// Wait for the plugin reference to be ready.
			pluginRef, data, _ := s.pluginFsTracker.AddKeyRef(pluginID)

			// Await the filesystem services for the plugin.
			res, err := data.resultPromiseCtr.Await(ctx)
			if err != nil {
				pluginRef.Release()
				return nil, nil, err
			}

			// Select the mux for the requested filesystem prefix.
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

			// Return the release function.
			return mux, pluginRef.Release, nil
		},
	)
}

// HostBusInvoker returns the invoker for the calls the plugin makes to
// services its mux does not register. allowedServiceIDs are host bus service
// IDs a plugin outside the host set may also call, matched exactly.
//
// Invariant: a plugin outside the host's set can call only the services the
// host registered for it and other plugins' services under its own binding
// key. No other service on the host bus is reachable from that plugin.
func (s *PluginHostServer) HostBusInvoker(allowedServiceIDs []string) srpc.Invoker {
	serverID := bldr_plugin.PluginServerID(s.pluginID, "")
	return srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		// A host plugin reaches the whole bus. Retained frontend grants exist
		// before their URLs are published and must fail after release, so
		// only those calls skip waiting for the service.
		if s.callerIsHostPlugin() {
			wait := !strings.HasPrefix(serviceID, bldr_frontend.AttachedServicePrefix)
			return bifrost_rpc.NewInvoker(s.b, serverID, wait).InvokeMethod(serviceID, methodID, strm)
		}

		// Any other plugin reaches the services allowed by exact ID.
		if slices.Contains(allowedServiceIDs, serviceID) {
			return bifrost_rpc.NewInvoker(s.b, serverID, true).InvokeMethod(serviceID, methodID, strm)
		}

		// Beyond those it reaches only other plugins' services.
		pluginID, remoteServiceID, ok := bldr_plugin.ParsePluginServiceID(serviceID)
		if !ok {
			s.le.Debugf("refused host bus service %q to plugin %q", serviceID, s.pluginID)
			return false, nil
		}

		// Deliver the call through the target's PluginRpc, as PluginRpc
		// does, so it arrives under "plugin/<caller>".
		caller, release, err := s.openPluginRpc(strm.Context(), pluginID, "", "")
		if err != nil {
			return true, err
		}
		defer release()
		client := rpcstream.NewRpcStreamClient(caller, s.caller().ComponentID(), false)
		return srpc.NewClientInvoker(client).InvokeMethod(remoteServiceID, methodID, strm)
	})
}

// caller returns the identity the host stamps on this server's plugin's calls
// to other plugins. It reads only fields set before the server is exposed.
func (s *PluginHostServer) caller() *bldr_plugin.PluginCaller {
	return &bldr_plugin.PluginCaller{
		PluginId:   s.pluginID,
		BindingKey: s.registrationInstanceKey,
		HostSet:    s.callerIsHostPlugin(),
	}
}

// callerIsHostPlugin reports whether this server's plugin is one the host
// itself starts: a member of the scheduler's host export list.
func (s *PluginHostServer) callerIsHostPlugin() bool {
	return slices.Contains(s.hostExportPluginIDs, s.pluginID)
}

// ExecController executes a config set on the host bus.
//
// Invariant: only plugins the host itself starts may run controllers on the
// host bus. A plugin loaded for a Space or approved by a person runs nothing
// there. Any other caller gets ErrNotHostPlugin before a config is resolved.
func (s *PluginHostServer) ExecController(
	req *controller_exec.ExecControllerRequest,
	strm bldr_plugin.SRPCPluginHost_ExecControllerStream,
) error {
	// Refuse a plugin the host did not start.
	if !s.callerIsHostPlugin() {
		s.le.Warnf("refused configset from plugin %q: %v", s.pluginID, bldr_plugin.ErrNotHostPlugin)
		return bldr_plugin.ErrNotHostPlugin
	}

	// Run the set on the host bus until the caller ends the stream.
	s.le.Debugf("plugin %q is applying a configset", s.pluginID)
	defer s.le.Debugf("plugin %q exited applying a configset", s.pluginID)
	return req.Execute(strm.Context(), s.b, true, strm.Send)
}

// WatchPluginStatus streams the host scheduler's plugin status.
func (s *PluginHostServer) WatchPluginStatus(
	req *bldr_plugin.WatchPluginStatusRequest,
	strm bldr_plugin.SRPCPluginHost_WatchPluginStatusStream,
) error {
	if s.scheduler == nil {
		return errors.New("plugin host has no scheduler")
	}
	instanceKey := s.scheduler.GetInstanceKey()
	return ccontainer.WatchChanges(
		strm.Context(),
		nil,
		s.scheduler.GetPluginStatusCtr(),
		func(status *bldr_plugin.PluginStatusSnapshot) error {
			return strm.Send(&bldr_plugin.WatchPluginStatusResponse{
				InstanceKey: instanceKey,
				Status:      status,
			})
		},
		nil,
	)
}

// _ is a type assertion
var _ bldr_plugin.SRPCPluginHostServer = (*PluginHostServer)(nil)
