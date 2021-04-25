package forge_api

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"google.golang.org/grpc"
)

// API implements the GRPC API.
type API struct {
	bus  bus.Bus
	conf *Config
}

// NewAPI constructs a new instance of the API.
func NewAPI(bus bus.Bus, conf *Config) (*API, error) {
	return &API{bus: bus, conf: conf}, nil
}

// RegisterAsGRPCServer registers the API to the GRPC instance.
func (a *API) RegisterAsGRPCServer(grpcServer *grpc.Server) {
	RegisterForgeDaemonServiceServer(grpcServer, a)
}

// _ is a type assertion
var _ ForgeDaemonServiceServer = ((*API)(nil))
