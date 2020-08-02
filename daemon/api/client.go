package hydra_api

import (
	bifrost_api "github.com/aperturerobotics/bifrost/daemon/api"
	"github.com/aperturerobotics/controllerbus/bus/api"
	"google.golang.org/grpc"
)

// HydraDaemonClient has all services provided by the daemon.
type HydraDaemonClient interface {
	HydraDaemonServiceClient
	bifrost_api.BifrostAPIClient
}

type hydraClient struct {
	HydraDaemonServiceClient
	bifrost_api.BifrostAPIClient
}

// NewHydraDaemonClient constructs a new hydra daemon client.
func NewHydraDaemonClient(cc *grpc.ClientConn) HydraDaemonClient {
	return &hydraClient{
		HydraDaemonServiceClient: NewHydraDaemonServiceClient(cc),
		BifrostAPIClient:         bifrost_api.NewBifrostAPIClient(cc),
	}
}

// _ is a type assertion
var _ bus_api.ControllerBusServiceClient = ((HydraDaemonClient)(nil))
