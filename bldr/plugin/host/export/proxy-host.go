package plugin_host_export

import (
	"context"
	"io"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	"github.com/s4wave/spacewave/db/unixfs"
)

// proxyHost executes plugins on one exported plugin host.
type proxyHost struct {
	// client reaches the exporting host
	client SRPCHostExportClient
	// platformID is the exported host's platform ID
	platformID string
	// root is the host resource root for plugins on this host
	root *plugin_host_root.Root
}

// newProxyHost constructs a proxyHost for platformID.
func newProxyHost(client SRPCHostExportClient, platformID string) *proxyHost {
	return &proxyHost{
		client:     client,
		platformID: platformID,
		root:       plugin_host_root.NewRoot(),
	}
}

// GetPlatformId returns the exported host's platform ID.
func (h *proxyHost) GetPlatformId() string {
	return h.platformID
}

// Execute returns nil: the exporting host runs its own lifecycle.
func (h *proxyHost) Execute(ctx context.Context) error {
	return nil
}

// ListPlugins returns nil: the exporting host owns its plugin list.
func (h *proxyHost) ListPlugins(ctx context.Context) ([]string, error) {
	return nil, nil
}

// ExecutePlugin runs the plugin on the exported host over one session. The
// session serves hostRpcMux to the plugin and carries calls to the plugin.
// pluginDist and pluginAssets are unused: the host reads them from
// hostRpcMux, which serves the same files.
func (h *proxyHost) ExecutePlugin(
	ctx context.Context,
	pluginID,
	instanceKey,
	executionKey,
	manifestRoot,
	entrypoint string,
	pluginDist *unixfs.FSHandle,
	pluginAssets *unixfs.FSHandle,
	hostRpcMux srpc.Mux,
	rpcInit plugin_host.PluginRpcInitCb,
) error {
	// Bound the proxied execution to the caller's lifetime.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Open the export stream and serve the caller's host mux over it.
	strm, err := h.client.ExecutePlugin(ctx)
	if err != nil {
		return err
	}
	defer strm.Close()
	mc, err := srpc.NewMuxedConnWithRwc(ctx, rpcstream.NewRpcStreamReadWriter(strm), true, nil)
	if err != nil {
		return err
	}
	defer mc.Close()
	go func() { _ = srpc.NewServer(hostRpcMux).AcceptMuxedConn(ctx, mc) }()

	// Start the plugin through the exported execution service.
	session := srpc.NewClientWithMuxedConn(mc)
	start, err := NewSRPCExecutionClient(session).Start(ctx, &StartRequest{
		PlatformId:   h.platformID,
		PluginId:     pluginID,
		InstanceKey:  instanceKey,
		ExecutionKey: executionKey,
		ManifestRoot: manifestRoot,
		Entrypoint:   entrypoint,
	})
	if err != nil {
		return err
	}
	defer start.Close()

	// Relay readiness until the execution ends, leaving the plugin not ready.
	var ready bool
	defer func() {
		if ready {
			_ = rpcInit(nil)
		}
	}()
	for {
		resp, err := start.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		ready = resp.GetRpcReady()
		client := session
		if !ready {
			client = nil
		}
		if err := rpcInit(client); err != nil {
			return errors.Wrap(err, "plugin rpc init")
		}
	}
}

// DeletePlugin returns nil: the exporting host owns its plugin data.
func (h *proxyHost) DeletePlugin(ctx context.Context, pluginID string) error {
	return nil
}

// _ is a type assertion
var _ plugin_host.PluginHost = (*proxyHost)(nil)
