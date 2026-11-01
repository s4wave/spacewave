package provider_local

import (
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/transport/inproc"
)

// newLocalSessionNetwork gives one native provider its own authenticated packet network.
func newLocalSessionNetwork() transport.SessionTransportOption {
	return transport.WithInprocNetwork(inproc.NewNetwork())
}
