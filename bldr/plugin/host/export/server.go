package plugin_host_export

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_client "github.com/s4wave/spacewave/db/unixfs/rpc/client"
	"github.com/sirupsen/logrus"
)

// HostSource reports the plugin hosts a Server exports.
type HostSource interface {
	// GetHostState returns the current hosts, a wait for them to change, and
	// the host lookup failure.
	GetHostState() ([]plugin_host.PluginHost, func(context.Context) error, error)
}

// Server exports a HostSource's plugin hosts to one plugin.
type Server struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus where executions publish their plugin files
	b bus.Bus
	// hosts reports the exported plugin hosts
	hosts HostSource
}

// NewServer constructs a Server. Executions publish their plugin files on b,
// where the hosts' runtimes look them up.
func NewServer(le *logrus.Entry, b bus.Bus, hosts HostSource) *Server {
	return &Server{le: le, b: b, hosts: hosts}
}

// WatchHosts streams the platform IDs of the exported hosts.
func (s *Server) WatchHosts(_ *WatchHostsRequest, strm SRPCHostExport_WatchHostsStream) error {
	ctx := strm.Context()
	var sent []string
	for first := true; ; first = false {
		hosts, wait, _ := s.hosts.GetHostState()
		platformIDs := make([]string, 0, len(hosts))
		for _, host := range hosts {
			platformIDs = append(platformIDs, host.GetPlatformId())
		}
		slices.Sort(platformIDs)
		platformIDs = slices.Compact(platformIDs)
		if first || !slices.Equal(platformIDs, sent) {
			if err := strm.Send(&WatchHostsResponse{PlatformIds: platformIDs}); err != nil {
				return err
			}
			sent = platformIDs
		}
		if err := wait(ctx); err != nil {
			return err
		}
	}
}

// ExecutePlugin serves one execution session until the caller closes it.
func (s *Server) ExecutePlugin(strm SRPCHostExport_ExecutePluginStream) error {
	ctx := strm.Context()
	mc, err := srpc.NewMuxedConnWithRwc(ctx, rpcstream.NewRpcStreamReadWriter(strm), false, nil)
	if err != nil {
		return err
	}
	defer mc.Close()

	exec := &execution{
		s:      s,
		caller: srpc.NewClientWithMuxedConn(mc),
		plugin: ccontainer.NewCContainer[srpc.Client](nil),
	}
	mux := srpc.NewMux(srpc.InvokerFunc(exec.invokePlugin))
	if err := SRPCRegisterExecution(mux, exec); err != nil {
		return err
	}
	return srpc.NewServer(mux).AcceptMuxedConn(ctx, mc)
}

// execution is the host side of one ExecutePlugin session.
type execution struct {
	// s is the exporting server
	s *Server
	// caller reaches the caller's host mux for the plugin
	caller srpc.Client
	// plugin is the running plugin's client, nil while not ready
	plugin *ccontainer.CContainer[srpc.Client]
}

// invokePlugin forwards a call from the caller to the running plugin.
func (e *execution) invokePlugin(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	client := e.plugin.GetValue()
	if client == nil {
		return false, nil
	}
	return srpc.NewClientInvoker(client).InvokeMethod(serviceID, methodID, strm)
}

// Start executes the plugin on the requested host and reports its readiness.
func (e *execution) Start(req *StartRequest, strm SRPCExecution_StartStream) error {
	ctx := strm.Context()
	hosts, _, err := e.s.hosts.GetHostState()
	i := slices.IndexFunc(hosts, func(host plugin_host.PluginHost) bool {
		return host.GetPlatformId() == req.GetPlatformId()
	})
	if i < 0 {
		if err != nil {
			return err
		}
		return errors.Errorf("no plugin host for platform %s", req.GetPlatformId())
	}
	host := hosts[i]

	// Read the plugin files through the caller's host mux.
	distFS, err := unixfs_rpc_client.BuildFSHandle(ctx, unixfs_rpc.NewSRPCFSCursorServiceClientWithServiceID(e.caller, bldr_plugin.PluginDistServiceID))
	if err != nil {
		return errors.Wrap(err, "open plugin dist")
	}
	defer distFS.Release()
	assetsFS, err := unixfs_rpc_client.BuildFSHandle(ctx, unixfs_rpc.NewSRPCFSCursorServiceClientWithServiceID(e.caller, bldr_plugin.PluginAssetsServiceID))
	if err != nil {
		return errors.Wrap(err, "open plugin assets")
	}
	defer assetsFS.Release()

	// Publish the immutable files where the host's runtime fetches them.
	artifactID := bldr_plugin.PluginArtifactID(req.GetPluginId(), req.GetManifestRoot())
	for _, files := range []struct {
		id string
		fs *unixfs.FSHandle
	}{
		{bldr_plugin.PluginDistFsId(artifactID), distFS},
		{bldr_plugin.PluginAssetsFsId(artifactID), assetsFS},
	} {
		ctrl := unixfs_access.NewController(e.s.le, e.s.b,
			controller.NewInfo(ControllerID+"/"+files.id, Version, "exported plugin files"),
			[]string{files.id}, unixfs_access.NewAccessUnixFSFunc(files.fs))
		defer ctrl.Close()
		release, err := e.s.b.AddController(ctx, ctrl, nil)
		if err != nil {
			return err
		}
		defer release()
	}

	// The plugin's calls to its host go to the caller.
	defer e.plugin.SetValue(nil)
	return host.ExecutePlugin(
		ctx,
		req.GetPluginId(),
		req.GetInstanceKey(),
		req.GetExecutionKey(),
		req.GetManifestRoot(),
		req.GetEntrypoint(),
		distFS,
		assetsFS,
		srpc.NewMux(srpc.NewClientInvoker(e.caller)),
		func(client srpc.Client) error {
			e.plugin.SetValue(client)
			return strm.Send(&StartResponse{RpcReady: client != nil})
		},
	)
}

// _ is a type assertion
var (
	_ SRPCHostExportServer = (*Server)(nil)
	_ SRPCExecutionServer  = (*execution)(nil)
)
