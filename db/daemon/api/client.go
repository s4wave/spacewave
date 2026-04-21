package hydra_api

import (
	bifrost_api "github.com/s4wave/spacewave/net/daemon/api"
	bus_api "github.com/aperturerobotics/controllerbus/bus/api"
	srpc "github.com/aperturerobotics/starpc/srpc"
)

// HydraDaemonClient has all services provided by the daemon.
type HydraDaemonClient interface {
	SRPCHydraDaemonServiceClient
	bifrost_api.BifrostAPIClient
}

type hydraClient struct {
	SRPCHydraDaemonServiceClient
	bifrost_api.BifrostAPIClient
	cc srpc.Client
}

// NewHydraDaemonClient constructs a new hydra daemon client.
func NewHydraDaemonClient(cc srpc.Client) HydraDaemonClient {
	return &hydraClient{
		SRPCHydraDaemonServiceClient: NewSRPCHydraDaemonServiceClient(cc),
		BifrostAPIClient:             bifrost_api.NewBifrostAPIClient(cc),
		cc:                           cc,
	}
}

// SRPCClient returns the srpc client.
func (c *hydraClient) SRPCClient() srpc.Client { return c.cc }

// _ is a type assertion
var _ bus_api.SRPCControllerBusServiceClient = ((HydraDaemonClient)(nil))
