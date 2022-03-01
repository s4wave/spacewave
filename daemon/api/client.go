package forge_api

import (
	bifrost_api "github.com/aperturerobotics/bifrost/daemon/api"
	bus_api "github.com/aperturerobotics/controllerbus/bus/api"
	hydra_api "github.com/aperturerobotics/hydra/daemon/api"
	"storj.io/drpc"
)

// ForgeDaemonClient has all services provided by the daemon.
type ForgeDaemonClient interface {
	DRPCForgeDaemonServiceClient
	bifrost_api.BifrostAPIClient
	hydra_api.DRPCHydraDaemonServiceClient
}

type forgeClient struct {
	DRPCForgeDaemonServiceClient
	bifrost_api.BifrostAPIClient
	hydra_api.DRPCHydraDaemonServiceClient
	cc drpc.Conn
}

// NewForgeDaemonClient constructs a new forge daemon client.
func NewForgeDaemonClient(cc drpc.Conn) ForgeDaemonClient {
	return &forgeClient{
		BifrostAPIClient:             bifrost_api.NewBifrostAPIClient(cc),
		DRPCForgeDaemonServiceClient: NewDRPCForgeDaemonServiceClient(cc),
		DRPCHydraDaemonServiceClient: hydra_api.NewDRPCHydraDaemonServiceClient(cc),
		cc:                           cc,
	}
}

func (c *forgeClient) DRPCConn() drpc.Conn { return c.cc }

// _ is a type assertion
var _ bus_api.DRPCControllerBusServiceClient = ((ForgeDaemonClient)(nil))
