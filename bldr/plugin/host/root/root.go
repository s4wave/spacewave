package plugin_host_root

import (
	"github.com/aperturerobotics/starpc/srpc"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// Root is the process-lifetime Bldr plugin host resource root.
type Root struct {
	desktopTray *desktop_tray.DesktopTray
	mux         srpc.Invoker
}

// NewRoot constructs a new Root.
func NewRoot() *Root {
	r := &Root{
		desktopTray: desktop_tray.NewDesktopTray(),
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

// GetMux returns the rpc mux for the root resource.
func (r *Root) GetMux() srpc.Invoker {
	return r.mux
}
