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
	if req.GetDaemonOwner() {
		release := l.c.attachDaemonUpdateWatcher()
		defer release()
	}
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

// ApplyUpdate hands an app artifact to Electron or publishes a verified daemon
// artifact for the serving daemon to replace after its full idle claim.
func (l *LauncherServer) ApplyUpdate(
	ctx context.Context,
	req *desktop_update.ApplyUpdateRequest,
) (*desktop_update.ApplyUpdateResponse, error) {
	switch req.GetTarget() {
	case desktop_update.UpdateTarget_UPDATE_TARGET_DAEMON:
		if info := l.c.launcherInfoCtr.GetValue(); info != nil && info.GetDaemonUpdateState().GetPhase() == spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING {
			if !l.c.hasDaemonUpdateWatcher() {
				return nil, errors.New("daemon update owner watch is unavailable")
			}
			return &desktop_update.ApplyUpdateResponse{}, nil
		}
		state, err := l.c.prepareDaemonUpdate(ctx)
		if err != nil {
			l.c.setDaemonUpdateError(err)
			return nil, err
		}
		if err := l.c.setDaemonUpdateApplying(state); err != nil {
			l.c.setDaemonUpdateError(err)
			return nil, err
		}
		return &desktop_update.ApplyUpdateResponse{}, nil
	case desktop_update.UpdateTarget_UPDATE_TARGET_APP:
	default:
		return nil, errors.New("select an installed app or shared daemon update target")
	}
	stagedPath, err := l.c.prepareAppUpdate(ctx)
	if err != nil {
		l.c.setUpdateError(err)
		return nil, err
	}
	return &desktop_update.ApplyUpdateResponse{StagedPath: stagedPath}, nil
}

// ReportDaemonUpdateFailure records a failed accepted handoff while the old
// daemon is still serving. The selection comparison rejects stale reports.
func (l *LauncherServer) ReportDaemonUpdateFailure(
	_ context.Context,
	req *spacewave_launcher.ReportDaemonUpdateFailureRequest,
) (*spacewave_launcher.ReportDaemonUpdateFailureResponse, error) {
	if req.GetErrorMessage() == "" {
		return nil, errors.New("daemon update failure message is empty")
	}
	return &spacewave_launcher.ReportDaemonUpdateFailureResponse{
		Reported: l.c.setAcceptedDaemonUpdateError(req.GetSelection(), req.GetErrorMessage()),
	}, nil
}

// ClaimDaemonUpdate records the successful owner idle fence before its watch
// ends while the old bus drains.
func (l *LauncherServer) ClaimDaemonUpdate(
	_ context.Context,
	req *spacewave_launcher.ClaimDaemonUpdateRequest,
) (*spacewave_launcher.ClaimDaemonUpdateResponse, error) {
	return &spacewave_launcher.ClaimDaemonUpdateResponse{
		Claimed: l.c.claimAcceptedDaemonUpdate(req.GetSelection()),
	}, nil
}

// ReportDaemonUpdateWait publishes the work an accepted daemon update waits
// for. The selection comparison rejects reports from an older acceptance.
func (l *LauncherServer) ReportDaemonUpdateWait(
	_ context.Context,
	req *spacewave_launcher.ReportDaemonUpdateWaitRequest,
) (*spacewave_launcher.ReportDaemonUpdateWaitResponse, error) {
	return &spacewave_launcher.ReportDaemonUpdateWaitResponse{
		Reported: l.c.setDaemonUpdateWait(req.GetSelection(), req.GetOtherClients(), req.GetOtherServices()),
	}, nil
}

// RestartDaemonUpdateNow asks the waiting daemon to hand off without waiting
// for its other clients and services.
func (l *LauncherServer) RestartDaemonUpdateNow(
	context.Context,
	*spacewave_launcher.RestartDaemonUpdateNowRequest,
) (*spacewave_launcher.RestartDaemonUpdateNowResponse, error) {
	if err := l.c.setDaemonUpdateRestartNow(); err != nil {
		return nil, err
	}
	return &spacewave_launcher.RestartDaemonUpdateNowResponse{}, nil
}

// _ is a type assertion
var _ spacewave_launcher.SRPCLauncherServer = (*LauncherServer)(nil)
