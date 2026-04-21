package forge_api

import (
	bus_api "github.com/aperturerobotics/controllerbus/bus/api"
	"github.com/aperturerobotics/starpc/srpc"
	hydra_api "github.com/s4wave/spacewave/db/daemon/api"
	bifrost_api "github.com/s4wave/spacewave/net/daemon/api"
)

// ForgeDaemonClient has all services provided by the daemon.
type ForgeDaemonClient interface {
	SRPCForgeDaemonServiceClient
	bifrost_api.BifrostAPIClient
	hydra_api.SRPCHydraDaemonServiceClient
}

type forgeClient struct {
	SRPCForgeDaemonServiceClient
	bifrost_api.BifrostAPIClient
	hydra_api.SRPCHydraDaemonServiceClient
	cc srpc.Client
}

// NewForgeDaemonClient constructs a new forge daemon client.
func NewForgeDaemonClient(cc srpc.Client) ForgeDaemonClient {
	return &forgeClient{
		BifrostAPIClient:             bifrost_api.NewBifrostAPIClient(cc),
		SRPCForgeDaemonServiceClient: NewSRPCForgeDaemonServiceClient(cc),
		SRPCHydraDaemonServiceClient: hydra_api.NewSRPCHydraDaemonServiceClient(cc),
		cc:                           cc,
	}
}

func (c *forgeClient) SRPCClient() srpc.Client { return c.cc }

// _ is a type assertion
var _ bus_api.SRPCControllerBusServiceClient = ((ForgeDaemonClient)(nil))
