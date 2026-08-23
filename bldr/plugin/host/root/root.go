package plugin_host_root

import (
	"github.com/aperturerobotics/starpc/srpc"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	plugin_host_logs "github.com/s4wave/spacewave/bldr/plugin/host/logs"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// Root is the process-lifetime Bldr plugin host resource root.
// Root is the plugin-host root resource owning the tray and structured
// log hub.
type Root struct {
	// desktopTray serves the desktop tray.
	desktopTray *desktop_tray.DesktopTray
	// structuredLogs owns structured plugin log events.
	structuredLogs *plugin_host_logs.Hub
	// mux routes resource access to the owned services.
	mux srpc.Invoker
}

// NewRoot constructs a new Root.
func NewRoot() *Root {
	r := &Root{
		desktopTray:    desktop_tray.NewDesktopTray(),
		structuredLogs: plugin_host_logs.NewHub(),
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return desktop_tray.SRPCRegisterDesktopTrayResourceService(mux, r.desktopTray)
	})
	return r
}

// GetDesktopTray returns the process-lifetime desktop tray registry.
func (r *Root) GetDesktopTray() *desktop_tray.DesktopTray {
	return r.desktopTray
}

// GetStructuredLogs returns the process-lifetime structured log hub.
func (r *Root) GetStructuredLogs() *plugin_host_logs.Hub {
	return r.structuredLogs
}

// GetMux returns the rpc mux for the root resource.
func (r *Root) GetMux() srpc.Invoker {
	return r.mux
}
