package stream_srpc_client_controller

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/protocol"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bifrost/stream/srpc/client"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller mounts a bifrost stream srpc client to a bus.
type Controller struct {
	*bifrost_rpc.ClientController
	// b is the controller bus
	b bus.Bus
	// conf is the config
	conf *Config
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	b bus.Bus,
	conf *Config,
) (*Controller, error) {
	// note: checked in Validate()
	c := &Controller{
		b:    b,
		conf: conf,
	}

	serviceIdPrefixes := conf.GetServiceIdPrefixes()
	if len(serviceIdPrefixes) == 0 {
		// match all service ids
		serviceIdPrefixes = append(serviceIdPrefixes, "")
	}

	client, err := stream_srpc_client.NewClient(
		le,
		b,
		conf.GetClient(),
		protocol.ID(conf.GetProtocolId()),
	)
	if err != nil {
		return nil, err
	}

	c.ClientController = bifrost_rpc.NewClientController(
		le,
		b,
		controller.NewInfo(
			ControllerID,
			Version,
			"bifrost stream rpc client",
		),
		client,
		serviceIdPrefixes,
	)

	return c, nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
