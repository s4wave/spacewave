package electron

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	runtime_controller "github.com/s4wave/spacewave/bldr/web/runtime/controller"
	"github.com/sirupsen/logrus"
)

// ControllerID is the Electron runtime controller ID.
const ControllerID = "bldr/web/plugin/electron"

// Version is the API version.
var Version = controller.MustParseVersion("0.0.1")

// RuntimeID is the runtime identifier.
const RuntimeID = "electron"

// Controller owns one Electron process at a time and starts it only on desktop demand.
type Controller struct {
	// le is the controller logger.
	le *logrus.Entry
	// bus is the plugin controller bus.
	bus bus.Bus

	// electronPath is the executable path.
	electronPath string
	// workdirPath holds the private runtime pipe.
	workdirPath string
	// rendererPath is the Electron entrypoint.
	rendererPath string
	// runtimeUuid identifies the private runtime pipe.
	runtimeUuid string
	// extraElectronArgs are passed to the process.
	extraElectronArgs []string
	// electronInit configures the desktop shell.
	electronInit *ElectronInit

	// bcast guards demand, runtime, presence, generation, and launchErr.
	bcast broadcast.Broadcast
	// demand is true while one shell is requested or running.
	demand bool
	// runtime is the current Electron remote runtime, if constructed.
	runtime web_runtime.WebRuntime
	// presence retains the current shell result for existing and late subscribers.
	presence *ccontainer.CContainer[*bldr_web_plugin.WatchDesktopPresenceResponse]
	// generation identifies the current launch attempt.
	generation uint64
	// launchErr is the result of the most recent ended launch.
	launchErr error
	// run starts one shell and publishes its runtime; tests replace it with a fixture.
	run func(context.Context) error
}

// NewController constructs an idle Electron controller. The plugin can serve
// routes before OpenOrFocusMainWindow starts the process.
func NewController(
	le *logrus.Entry,
	b bus.Bus,
	electronPath, workdirPath, rendererPath,
	runtimeUuid string,
	extraElectronArgs []string,
	electronInit *ElectronInit,
) (*Controller, error) {
	r := &Controller{
		le:                le,
		bus:               b,
		electronPath:      electronPath,
		workdirPath:       workdirPath,
		rendererPath:      rendererPath,
		runtimeUuid:       runtimeUuid,
		extraElectronArgs: extraElectronArgs,
		electronInit:      electronInit,
	}
	r.run = r.runElectron
	return r, nil
}

// GetControllerInfo returns information about the controller.
func (r *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "Electron "+r.runtimeUuid)
}

// GetLogger returns the root log entry.
func (r *Controller) GetLogger() *logrus.Entry {
	return r.le
}

// GetBus returns the plugin controller bus.
func (r *Controller) GetBus() bus.Bus {
	return r.bus
}

// Execute waits for desktop demand, runs one shell, and returns to an idle
// state after its exit. A failed launch is reported to its callers.
func (r *Controller) Execute(ctx context.Context) error {
	for {
		// Wait for an open request without starting Electron on plugin load.
		var wait <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if !r.demand {
				wait = getWaitCh()
			}
		})
		if wait != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-wait:
			}
			continue
		}

		// Run this demand to process exit, then release only desktop presence.
		presence := ccontainer.NewCContainerVT(&bldr_web_plugin.WatchDesktopPresenceResponse{
			State: bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ACTIVE,
		})
		r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			r.presence = presence
		})
		err := r.run(ctx)
		ended := &bldr_web_plugin.WatchDesktopPresenceResponse{
			State: bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED,
		}
		if err != nil {
			ended.Error = err.Error()
		}

		// Publish the terminal result only after the shell and private runtime are joined.

		// Clear the runtime state and record the launch result.
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {

			// Reset the runtime state and record the terminal launch error.
			r.runtime = nil
			r.demand = false
			r.launchErr = err
			if err == nil {
				r.launchErr = errDesktopClosed
			}
			broadcast()
		})
		presence.SetValue(ended)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// OpenOrFocusMainWindow starts one Electron process on cold or warm demand and
// returns its generation after Electron main acknowledges the operation.
func (r *Controller) OpenOrFocusMainWindow(ctx context.Context, req *bldr_web_plugin.OpenOrFocusDesktopRequest) (uint64, error) {
	// Join the current launch or signal the idle controller to start one.
	var generation uint64

	// Signal demand and take the new generation under the broadcast lock.
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !r.demand {
			r.demand = true
			r.generation++
			r.launchErr = nil
			broadcast()
		}
		generation = r.generation
	})

	// Wait for its remote runtime or the launch result.
	var rt web_runtime.WebRuntime
	var presence *ccontainer.CContainer[*bldr_web_plugin.WatchDesktopPresenceResponse]
	for {
		var wait <-chan struct{}
		var launchErr error

		// Read the launch state under the broadcast lock.
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {

			// Read the current launch error, runtime, and wait channel.
			launchErr = r.launchErr
			if r.generation != generation {
				launchErr = errDesktopClosed
				return
			}
			rt = r.runtime
			presence = r.presence
			if rt == nil && r.demand {
				wait = getWaitCh()
			}
		})
		if rt != nil {
			break
		}
		if wait == nil {
			return 0, launchErr
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-wait:
		}
	}

	// Cancel the open operation when this shell's private pipe ends.
	opCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		_, err := presence.WaitValueWithValidator(opCtx, func(state *bldr_web_plugin.WatchDesktopPresenceResponse) (bool, error) {
			return state.GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED, nil
		}, nil)
		if err == nil {
			cancel()
		}
	}()

	// Wait for Electron main to publish the desktop Resource service.
	if err := rt.WaitReady(opCtx); err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if presence.GetValue().GetState() == bldr_web_plugin.DesktopPresenceState_DESKTOP_PRESENCE_STATE_ENDED {
			return 0, errDesktopClosed
		}
		return 0, err
	}

	// Open a Resource client on the existing private runtime pipe.
	resources, err := rt.ConnectDesktopRuntimeResourceClient(opCtx)
	if err != nil {
		return 0, err
	}
	defer resources.Release()

	// Resolve the Electron main Resource service from its root reference.
	rootRef := resources.AccessRootResource()
	defer rootRef.Release()
	client, err := rootRef.GetClient()
	if err != nil {
		return 0, err
	}

	// Acknowledge only after Electron main has opened or focused the shell.
	service := desktop_runtime.NewSRPCDesktopRuntimeResourceServiceClient(client)
	_, err = service.OpenOrFocusMainWindow(opCtx, &desktop_runtime.OpenOrFocusMainWindowRequest{
		Route:        req.GetRoute(),
		InstalledApp: req.GetInstalledApp(),
	})
	if err != nil {
		return 0, err
	}
	return generation, nil
}

// DesktopPresence returns the owner's observation of one shell generation.
// A captured container keeps that generation's terminal result across warm reopen.
func (r *Controller) DesktopPresence(generation uint64) *ccontainer.CContainer[*bldr_web_plugin.WatchDesktopPresenceResponse] {
	var presence *ccontainer.CContainer[*bldr_web_plugin.WatchDesktopPresenceResponse]
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if r.generation == generation {
			presence = r.presence
		}
	})
	return presence
}

// runElectron runs one Electron process and its existing private pipe runtime.
func (r *Controller) runElectron(ctx context.Context) error {
	// Retain the process until its private runtime has stopped and the process is joined.
	e, err := RunElectron(ctx, r.le, r.electronPath, r.workdirPath, r.rendererPath,
		r.runtimeUuid, r.extraElectronArgs, r.electronInit)
	if err != nil {
		return err
	}
	defer e.Close()

	// Construct the established runtime controller on the plugin bus.
	rc := runtime_controller.NewController(r.le, r.bus,
		func(ctx context.Context, le *logrus.Entry, handler web_runtime.WebRuntimeHandler) (web_runtime.WebRuntime, error) {
			// Connect the runtime through Electron's private muxed pipe.
			mc := e.GetMuxedConn()
			client := srpc.NewClientWithMuxedConn(mc)
			remote, err := web_runtime.NewRemote(r.le, r.bus, handler, r.runtimeUuid, client,
				func(ctx context.Context, remote *web_runtime.Remote) error {
					return remote.GetRpcServer().AcceptMuxedConn(ctx, mc)
				})
			if err != nil {
				return nil, err
			}

			// Mirror the desktop tray only for the configured background policy.
			var rt web_runtime.WebRuntime = remote
			if r.hasTrayBackgroundPresence() {
				rt = &desktopTrayMirroredRuntime{WebRuntime: remote, controller: r}
			}

			// Publish this process's runtime to all waiting open requests.
			r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				r.runtime = rt
				broadcast()
			})
			return rt, nil
		}, ControllerID, Version)

	// Run the private runtime until it, Electron, or the owning controller ends.
	return r.executeRuntimeController(ctx, e, rc)
}

// executeRuntimeController detaches the private runtime after canceling its
// execution context, including when the bus has not attached it yet.
func (r *Controller) executeRuntimeController(ctx context.Context, e *Electron, rc controller.Controller) error {
	// Give runtime removal an explicit cancellation and completion fence.
	runCtx, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() { result <- r.bus.ExecuteController(runCtx, rc) }()

	// Select the first lifetime to end, then stop and join runtime execution.
	var err error
	var joined bool
	select {
	case err = <-result:
		joined = true
	case <-e.waitDone:
		err = e.waitErr
	case <-ctx.Done():
		err = ctx.Err()
	}
	cancel()
	r.bus.RemoveController(rc)
	if !joined {
		<-result
	}
	return err
}

// HandleDirective resolves the plugin's desktop control without starting Electron.
func (r *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	if _, ok := di.GetDirective().(bldr_web_plugin.LookupDesktop); ok {
		return directive.R(directive.NewValueResolver([]bldr_web_plugin.Desktop{r}), nil)
	}
	return nil, nil
}

// Close closes the controller after its Execute context is canceled.
func (r *Controller) Close() error {
	return nil
}

// _ is a type assertion.
var _ controller.Controller = (*Controller)(nil)
var _ bldr_web_plugin.Desktop = (*Controller)(nil)
