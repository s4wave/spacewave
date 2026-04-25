package spacewave_launcher_controller

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// LauncherServer implements the launcher service server.
type LauncherServer struct {
	c *Controller
}

// NewLauncherServer constructs a new LauncherServer with a controller.
func NewLauncherServer(c *Controller) *LauncherServer {
	return &LauncherServer{c: c}
}

// WatchLauncherInfo returns the current state of the launcher.
//
// Watches the state of the launcher and returns a stream.
func (l *LauncherServer) WatchLauncherInfo(
	req *spacewave_launcher.WatchLauncherInfoRequest,
	strm spacewave_launcher.SRPCLauncher_WatchLauncherInfoStream,
) error {
	return ccontainer.WatchChanges[*spacewave_launcher.LauncherInfo](strm.Context(), nil, l.c.launcherInfoCtr, strm.Send, nil)
}

// PushDistConfigMsg pushes a signed packedmsg with an DistConfig.
func (l *LauncherServer) PushDistConfigMsg(
	ctx context.Context,
	req *spacewave_launcher.PushDistConfigRequest,
) (*spacewave_launcher.PushDistConfigResponse, error) {
	foundConf, _, _, updated, prevRev, err := l.c.PushDistConf(ctx, []byte(req.GetBody()))
	if err != nil {
		return nil, err
	}
	return &spacewave_launcher.PushDistConfigResponse{
		Valid:   foundConf != nil,
		Updated: updated,
		Rev:     foundConf.GetRev(),
		PrevRev: prevRev,
	}, nil
}

// RecheckDistConfig triggers an immediate re-fetch of the app dist config.
func (l *LauncherServer) RecheckDistConfig(
	ctx context.Context,
	req *spacewave_launcher.RecheckDistConfigRequest,
) (*spacewave_launcher.RecheckDistConfigResponse, error) {
	l.c.RecheckDistConfig()
	return &spacewave_launcher.RecheckDistConfigResponse{}, nil
}

// ApplyUpdate applies a staged entrypoint update.
// Replaces the current binary and relaunches the process.
func (l *LauncherServer) ApplyUpdate(
	ctx context.Context,
	req *spacewave_launcher.ApplyUpdateRequest,
) (*spacewave_launcher.ApplyUpdateResponse, error) {
	if err := l.c.applyUpdate(); err != nil {
		return nil, err
	}
	return &spacewave_launcher.ApplyUpdateResponse{}, nil
}

// _ is a type assertion
var _ spacewave_launcher.SRPCLauncherServer = ((*LauncherServer)(nil))
