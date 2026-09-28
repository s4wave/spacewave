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
	"github.com/s4wave/spacewave/core/daemon"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// daemonUpdateHandoff carries both immutable executables beyond listener drain.
type daemonUpdateHandoff struct {
	selected string
	fallback string
}

// watchDaemonUpdate retains accepted intent beyond its short launcher RPC.
// A disposed launcher service is reacquired through its bus owner directive.
func watchDaemonUpdate(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	statePath string,
	idle *daemonIdleTracker,
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

		// The stream starts with the owner's current snapshot, so an acceptance
		// preceding registration is still observed.
		var observed *spacewave_launcher.UpdateState
		var handoff *daemonUpdateHandoff
		var watchErr error
		for {
			info, recvErr := stream.Recv()
			if recvErr != nil {
				watchErr = recvErr
				break
			}
			observed = info.GetDaemonUpdateState().CloneVT()
			if observed.GetPhase() != spacewave_launcher.UpdatePhase_UPDATE_PHASE_APPLYING {
				continue
			}

			le.Info("daemon update accepted; preparing selected and fallback CLI artifacts")
			handoff, err = prepareDaemonUpdateHandoff(statePath, observed)
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
					le.WithError(failure).Warn("daemon update rejected before idle claim")
				}
				continue
			}
			le.Info("daemon update waiting for all clients and services to finish")
			if !idle.waitDaemonUpdate(ctx) {
				if err := ctx.Err(); err != nil {
					watchErr = err
				} else {
					watchErr = errors.New("daemon stopped before update reached idle")
				}
				break
			}
			claimCtx, claimCancel := context.WithTimeout(serviceCtx, ownerRPCTimeout)
			claimed, claimErr := client.ClaimDaemonUpdate(claimCtx, &spacewave_launcher.ClaimDaemonUpdateRequest{Selection: observed})
			claimCancel()
			if claimErr != nil || !claimed.GetClaimed() {
				le.WithError(claimErr).Warn("launcher could not record daemon idle claim")
			}
			le.Info("daemon update claimed full idle; draining old listener")
			break
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
		if handoff != nil && watchErr == nil {
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

// prepareDaemonUpdateHandoff preserves the running CLI before claiming idle.
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
