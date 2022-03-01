package forge_api

import (
	"github.com/aperturerobotics/controllerbus/bus"
	drpc "storj.io/drpc"
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

// RegisterAsDRPCServer registers the API to the DRPC Mux.
func (a *API) RegisterAsDRPCServer(mux drpc.Mux) {
	DRPCRegisterForgeDaemonService(mux, a)
}

// _ is a type assertion
var _ DRPCForgeDaemonServiceServer = ((*API)(nil))
