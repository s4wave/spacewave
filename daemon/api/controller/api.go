package hydra_api_controller

import (
	"github.com/aperturerobotics/controllerbus/bus"
	api "github.com/aperturerobotics/hydra/daemon/api"
	"google.golang.org/grpc"
)

// API implements the GRPC API.
type API struct {
	bus bus.Bus
}

// NewAPI constructs a new instance of the API.
func NewAPI(bus bus.Bus) (*API, error) {
	return &API{bus: bus}, nil
}

// RegisterAsGRPCServer registers the API to the GRPC instance.
func (a *API) RegisterAsGRPCServer(grpcServer *grpc.Server) {
	api.RegisterHydraDaemonServiceServer(grpcServer, a)
}

// _ is a type assertion
var _ api.HydraDaemonServiceServer = ((*API)(nil))
