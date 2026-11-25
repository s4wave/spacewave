package os_system

import (
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	os_system_stager "github.com/s4wave/spacewave/os/system/stager"
)

// invoker routes the os.system services to the node placed on a bus.
type invoker struct {
	// inv looks each service up on the bus, waiting for it.
	inv srpc.Invoker
}

// NewInvoker routes each os.system service call to the os/system node placed
// on b, waiting until a node serves it, and leaves every other service to the
// next invoker. A daemon socket's mux takes it as a fallback, so the console
// reaches the node through the daemon it supervises.
func NewInvoker(b bus.Bus) srpc.Invoker {
	return &invoker{inv: bifrost_rpc.NewInvoker(b, "", true)}
}

// InvokeMethod invokes an os.system method on the placed node.
func (i *invoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	if !strings.HasPrefix(serviceID, os_system_stager.ServicePrefix) {
		return false, nil
	}
	return i.inv.InvokeMethod(serviceID, methodID, strm)
}

// _ is a type assertion
var _ srpc.Invoker = (*invoker)(nil)
