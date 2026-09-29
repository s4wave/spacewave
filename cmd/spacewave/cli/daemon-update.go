//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	desktop_control "github.com/s4wave/spacewave/bldr/desktop/control"
	"github.com/s4wave/spacewave/core/daemon"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// daemonUpdateHandoff carries both immutable executables beyond listener drain.
type daemonUpdateHandoff struct {
	selected string
	fallback string
	// reopen reopens the desktop shell closed by the handoff, if any.
	reopen *desktop_control.OpenOrFocusDesktopRequest
}

// watchDaemonUpdate retains accepted intent beyond its short launcher RPC.
// While an accepted update waits for other clients and services, it reports
// them to the launcher and claims the handoff when they finish or the user
// asks to restart now. A disposed launcher service is reacquired through its
// bus owner directive.
func watchDaemonUpdate(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	statePath string,
	idle *daemonIdleTracker,
	desktop *daemonDesktopControl,
) (*daemonUpdateHandoff, error) {
	const ownerRPCTimeout = 5 * time.Second
	for {
		// The lookup waits for the plugin service rather than probing for it.
		serviceCtx, serviceCancel := context.WithCancel(ctx)
		disposed := make(chan struct{})
		var disposeOnce sync.Once
		invokers, _, ref, err := bifrost_rpc.ExLookupRpcService(ctx, b, spacewave_launcher.PluginLauncherServiceID, "", true, func() {
			disposeOnce.Do(func() {
				close(disposed)
				serviceCancel()
			})
		})
		if err != nil {
			serviceCancel()
			return nil, err
		}
		if len(invokers) == 0 {
			serviceCancel()
			return nil, errors.New("launcher update service unavailable")
		}
		client := spacewave_launcher.NewSRPCLauncherClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0]))))
		stream, err := client.WatchLauncherInfo(serviceCtx, &spacewave_launcher.WatchLauncherInfoRequest{DaemonOwner: true})
		if err != nil {
			ref.Release()
			serviceCancel()
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			select {
			case <-disposed:
				continue
			default:
			}
			return nil, errors.Wrap(err, "open launcher update watch")
		}
		le.Info("daemon update watcher attached to launcher state")

		// Receive launcher snapshots alongside idle tracker changes. The stream
		// starts with the owner's current snapshot, so an acceptance preceding
		// registration is still observed.
		infos := make(chan *spacewave_launcher.LauncherInfo)
		recvErr := make(chan error, 1)
		go func() {
			for {
				info, err := stream.Recv()
				if err != nil {
					recvErr <- err
					return
				}
				select {
				case infos <- info:
				case <-serviceCtx.Done():
					return
				}
			}
		}()

		var observed *spacewave_launcher.UpdateState
		var prepared, handoff *daemonUpdateHandoff
		var restartNow bool
		var reported *daemonIdleSnapshot
		var watchErr error
		for handoff == nil && watchErr == nil {
			// Claim the handoff once the other work finishes or restart is requested.
			var idleChanged <-chan struct{}
			if prepared != nil {
				claimed, others, changed := idle.claimDaemonUpdate(restartNow)
				if claimed {
					handoff = prepared
					if others.desktop {
						handoff.reopen = desktop.reopenRequest()
					}
					claimCtx, claimCancel := context.WithTimeout(serviceCtx, ownerRPCTimeout)
					resp, claimErr := client.ClaimDaemonUpdate(claimCtx, &spacewave_launcher.ClaimDaemonUpdateRequest{Selection: observed})
					claimCancel()
					if claimErr != nil || !resp.GetClaimed() {
						le.WithError(claimErr).Warn("launcher could not record daemon update claim")
					}
					le.WithField("restart-now", restartNow).Info("daemon update claimed; draining old listener")
					break
				}
				if others.stopping {
					watchErr = errors.New("daemon stopped before update handoff")
					break
				}
				idleChanged = changed

				// Publish the other work the handoff waits for.
				if reported == nil || reported.clients != others.clients || reported.services != others.services {
					reportCtx, reportCancel := context.WithTimeout(serviceCtx, ownerRPCTimeout)
					_, reportErr := client.ReportDaemonUpdateWait(reportCtx, &spacewave_launcher.ReportDaemonUpdateWaitRequest{
						Selection:     observed,
						OtherClients:  uint32(others.clients),  // #nosec G115 -- hold counts are non-negative
						OtherServices: uint32(others.services), // #nosec G115 -- hold counts are non-negative
					})
					reportCancel()
					if reportErr != nil {
						le.WithError(reportErr).Warn("could not report daemon update wait")
					}
					reported = &others
				}
			}

			select {
			case <-ctx.Done():
				watchErr = ctx.Err()
			case err := <-recvErr:
				watchErr = err
			case <-idleChanged:
			case info := <-infos:
				state := info.GetDaemonUpdateState()
				if state.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING {
					observed, prepared, restartNow, reported = state.CloneVT(), nil, false, nil
					continue
				}
				restartNow = info.GetDaemonUpdateWait().GetRestartNow()
				if prepared != nil && state.EqualVT(observed) {
					continue
				}

				// Prepare both executables before waiting for the handoff.
				observed, prepared, reported = state.CloneVT(), nil, nil
				le.Info("daemon update accepted; preparing selected and fallback executables")
				next, err := prepareDaemonUpdateHandoff(statePath, observed)
				if err != nil {
					failure := errors.Wrap(err, "prepare accepted daemon update")
					reportCtx, reportCancel := context.WithTimeout(serviceCtx, ownerRPCTimeout)
					response, reportErr := client.ReportDaemonUpdateFailure(reportCtx, &spacewave_launcher.ReportDaemonUpdateFailureRequest{
						Selection:    observed,
						ErrorMessage: failure.Error(),
					})
					reportCancel()
					if reportErr != nil {
						watchErr = errors.Wrapf(reportErr, "report daemon update failure (%v)", failure)
						break
					}
					if response.GetReported() {
						le.WithError(failure).Warn("daemon update rejected before handoff claim")
					}
					continue
				}
				prepared = next
				le.Info("daemon update waiting for other clients and services to finish")
			}
		}

		// A failed stream may have missed acceptance. Report against the last
		// observed selection, then follow a new owner generation if disposed.
		if watchErr != nil && ctx.Err() == nil && observed != nil && observed.GetStagedSha256() != "" {
			reportCtx, reportCancel := context.WithTimeout(serviceCtx, ownerRPCTimeout)
			_, reportErr := client.ReportDaemonUpdateFailure(reportCtx, &spacewave_launcher.ReportDaemonUpdateFailureRequest{
				Selection:    observed,
				ErrorMessage: errors.Wrap(watchErr, "daemon update watch ended").Error(),
			})
			reportCancel()
			if reportErr != nil {
				le.WithError(reportErr).Warn("could not report launcher watch failure")
			}
		}
		_ = stream.Close()
		ref.Release()
		serviceCancel()
		if handoff != nil {
			return handoff, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		select {
		case <-disposed:
			continue
		default:
		}
		return nil, errors.Wrap(watchErr, "launcher update stream ended without owner change")
	}
}

// reopenDesktop asks the replacement daemon to open the desktop shell that
// the handoff closed. It never starts another daemon.
func reopenDesktop(ctx context.Context, statePath string, req *desktop_control.OpenOrFocusDesktopRequest) error {
	connector := daemon.NewConnector(nil, func(context.Context, string) error {
		return errors.New("replacement daemon is not running")
	})
	client, err := connector.Connect(ctx, statePath, "")
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = desktop_control.NewSRPCDesktopControlServiceClient(client.RPC()).OpenOrFocusDesktop(ctx, req)
	return err
}

// prepareDaemonUpdateHandoff preserves the running executable before the
// handoff claim.
func prepareDaemonUpdateHandoff(statePath string, selected *spacewave_launcher.UpdateState) (*daemonUpdateHandoff, error) {
	oldSource, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "locate running daemon executable")
	}
	fallback, err := daemon.PrepareExecutable(statePath, oldSource)
	if err != nil {
		return nil, errors.Wrap(err, "preserve running daemon executable")
	}
	executable, err := daemon.PrepareVerifiedExecutable(statePath, selected.GetStagedPath(), selected.GetStagedSha256())
	if err != nil {
		return nil, err
	}
	return &daemonUpdateHandoff{selected: executable, fallback: fallback}, nil
}
