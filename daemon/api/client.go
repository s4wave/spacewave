package forge_api

import (
	bifrost_api "github.com/aperturerobotics/bifrost/daemon/api"
	"github.com/aperturerobotics/controllerbus/bus/api"
	hydra_api "github.com/aperturerobotics/hydra/daemon/api"
	"google.golang.org/grpc"
)

// ForgeDaemonClient has all services provided by the daemon.
type ForgeDaemonClient interface {
	ForgeDaemonServiceClient
	bifrost_api.BifrostAPIClient
	hydra_api.HydraDaemonServiceClient
}

type forgeClient struct {
	ForgeDaemonServiceClient
	bifrost_api.BifrostAPIClient
	hydra_api.HydraDaemonServiceClient
}

// NewForgeDaemonClient constructs a new forge daemon client.
func NewForgeDaemonClient(cc *grpc.ClientConn) ForgeDaemonClient {
	return &forgeClient{
		BifrostAPIClient:         bifrost_api.NewBifrostAPIClient(cc),
		ForgeDaemonServiceClient: NewForgeDaemonServiceClient(cc),
		HydraDaemonServiceClient: hydra_api.NewHydraDaemonServiceClient(cc),
	}
}

// _ is a type assertion
var _ bus_api.ControllerBusServiceClient = ((ForgeDaemonClient)(nil))
