package forge_api

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
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

// RegisterAsSRPCServer registers the API to the SRPC Mux.
func (a *API) RegisterAsSRPCServer(mux srpc.Mux) {
	SRPCRegisterForgeDaemonService(mux, a)
}

// _ is a type assertion
var _ SRPCForgeDaemonServiceServer = ((*API)(nil))
