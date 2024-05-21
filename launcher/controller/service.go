package bldr_launcher_controller

import (
	"context"

	aperture_launcher "github.com/aperturerobotics/bldr/launcher"
	"github.com/aperturerobotics/util/ccontainer"
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
	req *aperture_launcher.WatchLauncherInfoRequest,
	strm aperture_launcher.SRPCLauncher_WatchLauncherInfoStream,
) error {
	return ccontainer.WatchChanges[*aperture_launcher.LauncherInfo](strm.Context(), nil, l.c.launcherInfoCtr, strm.Send, nil)
}

// PushDistConfigMsg pushes a signed packedmsg with an DistConfig.
func (l *LauncherServer) PushDistConfigMsg(
	ctx context.Context,
	req *aperture_launcher.PushDistConfigRequest,
) (*aperture_launcher.PushDistConfigResponse, error) {
	foundConf, _, _, updated, prevRev, err := l.c.PushDistConf(ctx, []byte(req.GetBody()))
	if err != nil {
		return nil, err
	}
	return &aperture_launcher.PushDistConfigResponse{
		Valid:   foundConf != nil,
		Updated: updated,
		Rev:     foundConf.GetRev(),
		PrevRev: prevRev,
	}, nil
}

// RecheckDistConfig triggers an immediate re-fetch of the app dist config.
func (l *LauncherServer) RecheckDistConfig(
	ctx context.Context,
	req *aperture_launcher.RecheckDistConfigRequest,
) (*aperture_launcher.RecheckDistConfigResponse, error) {
	l.c.mtx.Lock()
	if l.c.confFetcherRefetch != nil {
		_ = l.c.confFetcherRefetch.Stop()
		l.c.confFetcherRefetch = nil
	}
	_ = l.c.confFetcherRoutine.RestartRoutine()
	l.c.mtx.Unlock()
	return &aperture_launcher.RecheckDistConfigResponse{}, nil
}

// _ is a type assertion
var _ aperture_launcher.SRPCLauncherServer = ((*LauncherServer)(nil))
