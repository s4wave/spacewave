package sobject_invite

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/net/protocol"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	"github.com/sirupsen/logrus"
)

// ProtocolID is the bifrost protocol ID for the SO invite handshake.
const ProtocolID = protocol.ID("alpha/so-invite")

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "alpha/so-invite/server"

// NewInviteController constructs an SO invite controller.
func NewInviteController(
	le *logrus.Entry,
	b bus.Bus,
	h Handlers,
	peerIDs []string,
) (*stream_srpc_server.Server, error) {
	// Construct the private invitation service.
	srv := &server{h: h}

	// Bind the invitation service to the stream controller.
	return stream_srpc_server.NewServer(
		b,
		le,
		controller.NewInfo(ControllerID, Version, "so invite server"),
		[]stream_srpc_server.RegisterFn{
			func(mux srpc.Mux) error {
				return SRPCRegisterSOInviteService(mux, srv)
			},
		},
		[]protocol.ID{ProtocolID},
		peerIDs,
		false,
	)
}
