package statusprojector

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

type applyUpdateTrayActionHandler struct {
	bus bus.Bus
}

func (h *applyUpdateTrayActionHandler) HandleDesktopTrayAction(
	ctx context.Context,
	req *desktop_tray.HandleDesktopTrayActionRequest,
) (*desktop_tray.HandleDesktopTrayActionResponse, error) {
	invokers, _, invokerRef, err := bifrost_rpc.ExLookupRpcService(
		ctx,
		h.bus,
		spacewave_launcher.SRPCLauncherServiceID,
		"",
		true,
		nil,
	)
	if err != nil {
		return nil, err
	}
	if invokerRef != nil {
		defer invokerRef.Release()
	}
	if len(invokers) == 0 {
		return nil, errors.New("launcher service not found")
	}

	client := spacewave_launcher.NewSRPCLauncherClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0]))))
	if _, err := client.ApplyUpdate(ctx, &spacewave_launcher.ApplyUpdateRequest{}); err != nil {
		return nil, err
	}
	return &desktop_tray.HandleDesktopTrayActionResponse{}, nil
}

// _ is a type assertion
var _ desktop_tray.SRPCDesktopTrayActionHandlerServiceServer = (*applyUpdateTrayActionHandler)(nil)
