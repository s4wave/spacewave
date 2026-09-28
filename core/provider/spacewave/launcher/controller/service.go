package spacewave_launcher_controller

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	desktop_update "github.com/s4wave/spacewave/bldr/desktop/update"
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

// ApplyUpdate returns a verified desktop artifact to its app process. Daemon
// replacement requires a separate idle handoff and is not accepted here.
func (l *LauncherServer) ApplyUpdate(
	ctx context.Context,
	req *desktop_update.ApplyUpdateRequest,
) (*desktop_update.ApplyUpdateResponse, error) {
	if req.GetTarget() != desktop_update.UpdateTarget_UPDATE_TARGET_APP {
		return nil, errors.New("select the installed app; daemon updates are not available yet")
	}
	stagedPath, err := l.c.prepareAppUpdate(ctx)
	if err != nil {
		l.c.setUpdateError(err)
		return nil, err
	}
	return &desktop_update.ApplyUpdateResponse{StagedPath: stagedPath}, nil
}

// _ is a type assertion
var _ spacewave_launcher.SRPCLauncherServer = (*LauncherServer)(nil)
