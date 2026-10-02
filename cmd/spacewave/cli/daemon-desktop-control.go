//go:build !js

package spacewave_cli

import (
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	desktop_control "github.com/s4wave/spacewave/bldr/desktop/control"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	"github.com/s4wave/spacewave/core/appversion"
)

// ErrDesktopUIUnavailable reports that the running daemon has no web plugin
// to host the desktop. A launcher receives this exact message over RPC.
var ErrDesktopUIUnavailable = errors.New("desktop UI artifact unavailable: the running daemon has no web plugin")

// daemonDesktopControl serves desktop requests on the daemon's protected Resource socket.
// Its plugin reference and desktop demand belong to the daemon, not the calling connection.
type daemonDesktopControl struct {
	// ctx is the daemon lifecycle context retained by desktop state watches.
	ctx context.Context
	// load resolves the current web plugin and selected UI artifact.
	load func(context.Context) (bldr_web_plugin.SRPCWebPluginClient, string, func(), error)
	// idleTracker counts Electron presence outside the socket client count.
	idleTracker *daemonIdleTracker

	// bcast guards retained references and the latest owner observation.
	bcast broadcast.Broadcast
	// status is replayed to current and future desktop-status clients.
	status *desktop_control.WatchDesktopStatusResponse
	// closed prevents in-flight launcher requests from restoring demand after shutdown.
	closed bool
	// pluginRelease retains web plugin infrastructure for warm reopen.
	pluginRelease func()
	// demandRelease counts the current Electron shell as a daemon service.
	demandRelease func()
	// installedApp is the app bundle named by the latest open request.
	installedApp string
	// watchCancel stops the daemon-owned observation of the latest shell.
	watchCancel context.CancelFunc
	// watchClient identifies the web plugin instance that issued watchGeneration.
	watchClient srpc.Client
	// watchGeneration identifies the shell whose presence owns demand.
	watchGeneration uint64
	// watchSequence prevents an older shell watch from releasing newer demand.
	watchSequence uint64
	// quitRequester identifies the connection that requested Quit for watchSequence.
	quitRequester *trackedConn
	// shutdown closes admission after Quit wins the idle tracker's stop claim.
	shutdown func(*trackedConn)
}

// newDaemonDesktopControl binds desktop loading to the daemon's PluginHost.
func newDaemonDesktopControl(ctx context.Context, b bus.Bus, idleTracker *daemonIdleTracker) *daemonDesktopControl {
	return &daemonDesktopControl{
		ctx:         ctx,
		idleTracker: idleTracker,
		load: func(ctx context.Context) (bldr_web_plugin.SRPCWebPluginClient, string, func(), error) {
			// Let PluginHost finish selection, including a terminal idle result when no artifact exists.
			running, _, probeRef, err := bldr_plugin.ExLoadPlugin(ctx, b, true, "web", nil)
			if probeRef != nil {
				defer probeRef.Release()
			}
			if err != nil {
				return nil, "", nil, errors.Wrap(err, "desktop UI artifact unavailable; inspect the daemon plugin catalog")
			}
			if running == nil {
				return nil, "", nil, ErrDesktopUIUnavailable
			}

			// Load the shared web plugin and retain its reference until the daemon stops.
			client, ref, err := bldr_plugin.ExPluginLoadWaitClient(ctx, b, "web", nil)
			if err != nil {
				return nil, "", nil, errors.Wrap(err, "desktop UI artifact unavailable; install the matching web plugin for this daemon")
			}
			if ref == nil {
				return nil, "", nil, errors.New("desktop UI artifact unavailable; install the matching web plugin for this daemon")
			}

			// Read the selected artifact from the scheduler that loaded this plugin.
			artifact, err := selectedDesktopArtifact(b)
			if err != nil {
				ref.Release()
				return nil, "", nil, err
			}
			return bldr_web_plugin.NewSRPCWebPluginClient(client), artifact, ref.Release, nil
		},
	}
}

// selectedDesktopArtifact returns the web manifest selected for execution by PluginHost.
func selectedDesktopArtifact(b bus.Bus) (string, error) {
	// Read the scheduler that owns web plugin selection on this daemon bus.
	scheduler := plugin_host_scheduler.FindControllerOnBus(b)
	if scheduler == nil {
		return "", errors.New("desktop UI artifact unavailable: plugin scheduler is not running")
	}

	// Return the manifest selected for the shared web plugin instance.
	snapshot := scheduler.GetPluginStatusCtr().GetValue()
	for _, row := range snapshot.GetManifestRecovery() {
		if row.GetPluginId() == "web" && row.GetInstanceKey() == "" && row.GetExecuteManifestRef() != "" {
			return row.GetExecuteManifestRef(), nil
		}
	}
	return "", errors.New("desktop UI artifact unavailable: web plugin has no selected manifest")
}

// OpenOrFocusDesktop forwards one request to the daemon's web plugin and
// acknowledges only after Electron opens or focuses its main window.
func (d *daemonDesktopControl) OpenOrFocusDesktop(
	ctx context.Context,
	req *desktop_control.OpenOrFocusDesktopRequest,
) (*desktop_control.OpenOrFocusDesktopResponse, error) {
	// Identify the executable actually serving this socket before opening the desktop.
	executable, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "identify running daemon executable")
	}

	// Load the current web plugin generation without binding it to this socket client.
	client, artifact, release, err := d.load(ctx)
	if err != nil {
		d.reportFailure(err)
		return nil, err
	}
	retained := false
	defer func() {
		if !retained {
			release()
		}
	}()

	// Forward the request and wait for the Electron owner's open acknowledgement.
	opened, err := client.OpenOrFocusDesktop(ctx, &bldr_web_plugin.OpenOrFocusDesktopRequest{
		Route:        req.GetRoute(),
		InstalledApp: req.GetInstalledApp(),
	})
	if err != nil {
		err = errors.Wrap(err, "desktop capability or launch failed in the running daemon's web plugin; update its UI artifact or inspect the daemon log")
		d.reportFailure(err)
		return nil, err
	}

	// Transfer one plugin reference and persistent desktop demand to the daemon.
	retained, err = d.retainDesktop(ctx, client, opened.GetGeneration(), release)
	if err != nil {
		return nil, err
	}
	d.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		d.installedApp = req.GetInstalledApp()
	})

	// Return the daemon executable and UI manifest that served this request.
	return &desktop_control.OpenOrFocusDesktopResponse{
		DaemonPid:        int64(os.Getpid()),
		DaemonExecutable: executable,
		UiManifestRef:    artifact,
		DaemonRelease:    appversion.GetVersion(),
	}, nil
}

// QuitDesktop decides whether the daemon can stop while Electron can report
// other work. A winning claim fences admission before the shell exits.
func (d *daemonDesktopControl) QuitDesktop(
	ctx context.Context,
	_ *desktop_control.QuitDesktopRequest,
) (*desktop_control.QuitDesktopResponse, error) {
	// Bind this Quit to its admitted socket and the currently active shell.
	requester, ok := ctx.Value(daemonConnCtxKey{}).(*trackedConn)
	if !ok {
		return nil, errors.New("desktop Quit requires an admitted daemon connection")
	}
	var active bool
	var snapshot daemonIdleSnapshot
	var claimed bool
	d.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		active = !d.closed && d.demandRelease != nil &&
			d.status.GetPresence().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE
		if active {
			claimed, snapshot = d.idleTracker.claimDesktopQuit(requester)
			if claimed {
				d.quitRequester = requester
			}
		}
	})
	if !active {
		return nil, errors.New("desktop shell is not active")
	}

	// Return the final decision before Electron begins exiting.
	return &desktop_control.QuitDesktopResponse{
		OtherWork: snapshot.others,
	}, nil
}

// retainDesktop retains the acknowledged shell before observing its lifetime.
// Only an owner-confirmed ENDED result releases demand; observation failure does not.
func (d *daemonDesktopControl) retainDesktop(
	ctx context.Context,
	client bldr_web_plugin.SRPCWebPluginClient,
	generation uint64,
	release func(),
) (bool, error) {
	// Protect the acknowledged shell while transferring its demand to the daemon.
	hold := d.idleTracker.attachService("desktop app")
	if hold.released {
		return false, errors.New("desktop daemon is shutting down")
	}
	releaseDemand := hold.release
	var demandRetained bool
	var watchCtx context.Context
	var sequence uint64

	// Share one daemon-owned watch among requests for the same shell.
	var retained bool
	var err error
	var previousCancel context.CancelFunc
	d.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Reject open or watch requests while the daemon is shutting down.
		if d.closed || d.ctx.Err() != nil {
			err = errors.New("desktop daemon is shutting down")
			return
		}

		// Reject requests that reference a superseded shell generation.
		if d.watchClient == client.SRPCClient() && generation < d.watchGeneration {
			err = errors.New("desktop shell changed before acknowledgement; reopen the desktop")
			return
		}

		// Retain the first caller's plugin release reference.
		if d.pluginRelease == nil {
			d.pluginRelease = release
			retained = true
		}

		// Reuse the active watch for the same shell and clear a stale failure.
		if d.watchClient == client.SRPCClient() && d.watchGeneration == generation && d.watchCancel != nil {
			if d.status.GetFailure() != "" {
				d.status = d.status.CloneVT()
				d.status.Failure = ""
				broadcast()
			}
			return
		}

		// Retain demand even if the first presence receive fails after Electron opened.
		if d.demandRelease == nil {
			d.demandRelease = releaseDemand
			d.idleTracker.setDesktop(hold)
			demandRetained = true
		}

		// Take over the watch slot and start a fresh cancellation scope for the shell.
		previousCancel = d.watchCancel
		var cancel context.CancelFunc

		// #nosec G118 -- watchCancel owns cancellation until replacement, ENDED, observation failure, or close.
		watchCtx, cancel = context.WithCancel(d.ctx)
		d.watchCancel = cancel

		// Install the new shell's watch state and wake status watchers.
		d.watchClient = client.SRPCClient()
		d.watchGeneration = generation
		d.watchSequence++
		d.quitRequester = nil
		d.status = &desktop_control.WatchDesktopStatusResponse{Generation: generation}
		sequence = d.watchSequence
		broadcast()
	})

	// Replace the observation and release unused demand outside the shared lock.
	if previousCancel != nil {
		previousCancel()
	}
	if !demandRetained {
		releaseDemand()
	}
	if watchCtx != nil {
		go d.watchDesktop(watchCtx, sequence, client, generation)
		go d.waitDesktopExit(watchCtx, sequence, client, generation)
	}
	if err != nil {
		return retained, err
	}

	// A launcher waits for readiness, while observation continues after it disconnects.
	for {
		var status *desktop_control.WatchDesktopStatusResponse
		var wait <-chan struct{}
		var closed bool
		d.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			status = d.status
			closed = d.closed
			wait = getWaitCh()
		})
		if closed {
			return retained, errors.New("desktop daemon is shutting down")
		}
		if status.GetGeneration() != generation {
			return retained, errors.New("desktop shell changed before acknowledgement; reopen the desktop")
		}
		if status.GetFailure() != "" {
			return retained, errors.New(status.GetFailure())
		}
		switch status.GetPresence().GetState() {
		case bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE:
			return retained, nil
		case bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED:
			return retained, errors.New("desktop shell ended before acknowledgement; inspect the daemon log and reopen")
		}
		select {
		case <-ctx.Done():
			return retained, ctx.Err()
		case <-d.ctx.Done():
			return retained, d.ctx.Err()
		case <-wait:
		}
	}
}

// watchDesktop forwards owner observations and releases demand only on ENDED.
func (d *daemonDesktopControl) watchDesktop(
	ctx context.Context,
	sequence uint64,
	client bldr_web_plugin.SRPCWebPluginClient,
	generation uint64,
) {
	// Keep the private presence stream independent of any launcher or status client.
	stream, err := client.WatchDesktopPresence(ctx, &bldr_web_plugin.WatchDesktopPresenceRequest{Generation: generation})
	if err == nil {
		// Closing the local receive stream has no additional failure to handle.
		defer stream.Close()
		for {
			var state *bldr_web_plugin.WatchDesktopPresenceResponse
			state, err = stream.Recv()
			if err != nil {
				break
			}
			if state.GetState() != bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE &&
				state.GetState() != bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED {
				err = errors.New("desktop owner returned an unknown presence state")
				break
			}

			// Publish the owner's result and fence observations from superseded shells.
			if state.GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED {
				d.desktopEnded(sequence, generation, state)
				return
			}

			d.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if sequence != d.watchSequence || d.demandRelease == nil {
					return
				}
				d.status = &desktop_control.WatchDesktopStatusResponse{Generation: generation, Presence: state}
				broadcast()
			})
		}
	}

	// A broken stream cannot prove Electron exited; retain its service demand.
	d.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if sequence != d.watchSequence || d.demandRelease == nil {
			return
		}
		d.status = d.status.CloneVT()
		d.status.Failure = errors.Wrap(err, "desktop presence unavailable; inspect the daemon log and reopen to observe the shell").Error()
		broadcast()
	})
}

// waitDesktopExit obtains the owner's retained terminal state even when the
// status stream failed or this call reaches the owner after shell exit.
func (d *daemonDesktopControl) waitDesktopExit(ctx context.Context, sequence uint64, client bldr_web_plugin.SRPCWebPluginClient, generation uint64) {
	state, err := client.WaitDesktopExit(ctx, &bldr_web_plugin.WatchDesktopPresenceRequest{Generation: generation})
	if err != nil {
		return
	}
	d.desktopEnded(sequence, generation, state)
}

// desktopEnded releases the identified shell once after an owner-confirmed exit.
func (d *daemonDesktopControl) desktopEnded(sequence, generation uint64, state *bldr_web_plugin.WatchDesktopPresenceResponse) {
	// Collect the shell's owned references under the shared lock.
	var release func()
	var cancel context.CancelFunc
	var requester *trackedConn
	d.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {

		// Publish the owner's terminal presence for the current shell generation.
		if sequence != d.watchSequence || d.demandRelease == nil {
			return
		}
		d.status = &desktop_control.WatchDesktopStatusResponse{Generation: generation, Presence: state}

		// Take the shell's owned references before releasing them outside the lock.
		release = d.demandRelease
		d.demandRelease = nil
		cancel = d.watchCancel
		d.watchCancel = nil
		requester = d.quitRequester
		d.quitRequester = nil
		broadcast()
	})
	if cancel != nil {
		cancel()
	}
	if release != nil {
		release()
	}
	if requester != nil {
		d.shutdown(requester)
	}
}

// reportFailure makes a failed desktop request visible without changing shell presence.
func (d *daemonDesktopControl) reportFailure(err error) {
	d.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {

		// Ignore failure reports after the daemon stopped serving requests.
		if d.closed {
			return
		}

		// Clone the current status and attach the failure.
		status := &desktop_control.WatchDesktopStatusResponse{}
		if d.status != nil {
			status = d.status.CloneVT()
		}
		status.Failure = err.Error()
		d.status = status
		broadcast()
	})
}

// WatchDesktopStatus replays current status, then watches daemon-owned observations.
// Canceling this RPC only unsubscribes the caller; it cannot release desktop demand.
func (d *daemonDesktopControl) WatchDesktopStatus(
	_ *desktop_control.WatchDesktopStatusRequest,
	stream desktop_control.SRPCDesktopControlService_WatchDesktopStatusStream,
) error {
	var previous *desktop_control.WatchDesktopStatusResponse
	for {
		// Read the snapshot and its next notification under the same lock.
		var status *desktop_control.WatchDesktopStatusResponse
		var wait <-chan struct{}
		var closed bool
		d.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			status = d.status
			closed = d.closed
			wait = getWaitCh()
		})
		if closed {
			return context.Canceled
		}
		if status == nil {
			status = &desktop_control.WatchDesktopStatusResponse{}
		}

		// Send outside the lock so a slow subscriber cannot block the owner watch.
		if !status.EqualVT(previous) {
			if err := stream.Send(status); err != nil {
				return err
			}
			previous = status
		}
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-d.ctx.Done():
			return d.ctx.Err()
		case <-wait:
		}
	}
}

// reopenRequest returns the request that reopens the current desktop shell
// on a replacement daemon.
func (d *daemonDesktopControl) reopenRequest() *desktop_control.OpenOrFocusDesktopRequest {
	var req *desktop_control.OpenOrFocusDesktopRequest
	d.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		req = &desktop_control.OpenOrFocusDesktopRequest{InstalledApp: d.installedApp}
	})
	return req
}

// close releases desktop demand and the retained plugin when serve stops.
func (d *daemonDesktopControl) close() {
	// Remove daemon-owned references before running their release callbacks.
	var releaseDemand, releasePlugin func()
	var cancel context.CancelFunc
	d.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {

		// Mark the service closed and capture its owned references.
		d.closed = true
		releaseDemand = d.demandRelease
		releasePlugin = d.pluginRelease
		cancel = d.watchCancel

		// Clear the daemon's owned references and wake status watchers.
		d.demandRelease = nil
		d.pluginRelease = nil
		d.watchCancel = nil
		d.watchClient = nil
		d.watchGeneration = 0
		d.watchSequence++
		d.quitRequester = nil
		broadcast()
	})

	// Release the shell, plugin, and watch outside the service lock.
	if cancel != nil {
		cancel()
	}
	if releaseDemand != nil {
		releaseDemand()
	}
	if releasePlugin != nil {
		releasePlugin()
	}
}

// _ is a type assertion.
var _ desktop_control.SRPCDesktopControlServiceServer = (*daemonDesktopControl)(nil)
