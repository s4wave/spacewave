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

// InviteController wraps the SRPC server with the bifrost stream handler.
type InviteController struct {
	// Server owns the invitation stream service and its controller lifetime.
	*stream_srpc_server.Server
}

// NewInviteController constructs an SO invite controller.
func NewInviteController(
	le *logrus.Entry,
	b bus.Bus,
	lookupFn InviteLookupFn,
	enrollFn EnrollFn,
	leaveFn LeaveFn,
	peerIDs []string,
) (*InviteController, error) {
	// Construct the invitation stream server and its SRPC controller.
	srv := NewServer(le, lookupFn, enrollFn, leaveFn)
	ctrl := &InviteController{}
	var err error
	ctrl.Server, err = stream_srpc_server.NewServer(
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
	if err != nil {
		return nil, err
	}
	return ctrl, nil
}
