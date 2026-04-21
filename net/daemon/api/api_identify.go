package bifrost_api

import (
	"context"

	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	peer_api "github.com/s4wave/spacewave/net/peer/api"
)

// Identify loads and manages a private key identity.
func (a *API) Identify(
	req *peer_api.IdentifyRequest,
	serv peer_api.SRPCPeerService_IdentifyStream,
) error {
	ctx := serv.Context()
	conf := req.GetConfig()
	if err := conf.Validate(); err != nil {
		return err
	}

	reqCtx, reqCtxCancel := context.WithCancel(ctx)
	defer reqCtxCancel()

	return controller_exec.ExecuteController(
		reqCtx,
		a.bus,
		conf,
		func(status controller_exec.ControllerStatus) {
			_ = serv.Send(&peer_api.IdentifyResponse{
				ControllerStatus: status,
			})
		},
	)
}
