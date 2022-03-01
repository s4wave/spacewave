package hydra_api

import (
	bifrost_api "github.com/aperturerobotics/bifrost/daemon/api"
	bus_api "github.com/aperturerobotics/controllerbus/bus/api"
	"storj.io/drpc"
)

// HydraDaemonClient has all services provided by the daemon.
type HydraDaemonClient interface {
	DRPCHydraDaemonServiceClient
	bifrost_api.BifrostAPIClient
}

type hydraClient struct {
	DRPCHydraDaemonServiceClient
	bifrost_api.BifrostAPIClient
	cc drpc.Conn
}

// NewHydraDaemonClient constructs a new hydra daemon client.
func NewHydraDaemonClient(cc drpc.Conn) HydraDaemonClient {
	return &hydraClient{
		DRPCHydraDaemonServiceClient: NewDRPCHydraDaemonServiceClient(cc),
		BifrostAPIClient:             bifrost_api.NewBifrostAPIClient(cc),
		cc:                           cc,
	}
}

// DRPCConn returns the drpc connection.
func (c *hydraClient) DRPCConn() drpc.Conn { return c.cc }

// _ is a type assertion
var _ bus_api.DRPCControllerBusServiceClient = ((HydraDaemonClient)(nil))
