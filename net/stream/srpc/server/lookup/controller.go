package stream_srpc_server_lookup

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	"github.com/s4wave/spacewave/net/util/confparse"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bifrost/stream/srpc/server/lookup"

// Controller handles incoming HandleMountedStream directives and routes
// them to RPC services resolved via LookupRpcService on the bus.
type Controller = stream_srpc_server.Server

// NewController constructs a new lookup SRPC server controller.
// Incoming streams matching the protocol IDs are routed through an SRPC
// mux backed by bifrost_rpc.NewInvoker, which resolves services via the
// LookupRpcService directive on the bus.
func NewController(
	b bus.Bus,
	le *logrus.Entry,
	conf *Config,
) (*Controller, error) {
	protocolIDs, err := confparse.ParseProtocolIDs(conf.GetProtocolIds(), false)
	if err != nil {
		return nil, err
	}
	peerIDs, err := confparse.ParsePeerIDs(conf.GetPeerIds(), false)
	if err != nil {
		return nil, err
	}

	invoker := bifrost_rpc.NewInvoker(b, conf.GetServerId(), true)
	mux := srpc.NewMux(invoker)

	info := controller.NewInfo(ControllerID, Version, "lookup srpc server")
	return stream_srpc_server.NewServerWithMux(
		b,
		le,
		info,
		mux,
		protocolIDs,
		peer.IDsToString(peerIDs),
		false,
	), nil
}
