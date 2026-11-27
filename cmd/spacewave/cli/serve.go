//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	desktop_control "github.com/s4wave/spacewave/bldr/desktop/control"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_default "github.com/s4wave/spacewave/bldr/plugin/host/default"
	plugin_host_resource "github.com/s4wave/spacewave/bldr/plugin/host/resource"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/daemon"
	device_flowgraph "github.com/s4wave/spacewave/core/device/flowgraph"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	resource_root "github.com/s4wave/spacewave/core/resource/root"
	trace_service "github.com/s4wave/spacewave/core/trace/service"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	os_system "github.com/s4wave/spacewave/os/system"
	flowgraph_nodetype "github.com/s4wave/spacewave/sdk/flowgraph/nodetype"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

// desktopDaemonSocketEnvVar carries this daemon's exact protected socket to its Electron shell.
const desktopDaemonSocketEnvVar = "SPACEWAVE_DESKTOP_DAEMON_SOCKET_PATH"

// serveSocketPath selects the exact listener requested by the command and
// falls back to the state-local daemon socket.
func serveSocketPath(c *cli.Context, statePath string) string {
	return effectiveSocketPath(c, filepath.Join(statePath, socketName))
}

// newServeCommand builds the serve command that starts the daemon
// with a resource service socket listener. Its flags set frees and the format
// upgrade decision, which the bus's storage reads as each Volume opens.
func newServeCommand(
	getBus func() cli_entrypoint.CliBus,
	yieldBroker *yield_policy.Broker,
	frees *CollectorFrees,
	upgrade *FormatUpgrade,
	devicePolicy *device_policy.PolicyStore,
) *cli.Command {
	// Keep serve flags scoped to this command's startup and idle policy.
	var startupPipeID string
	var launcher string
	var runtimeTracePath string
	var udpListen string
	var udpPeers cli.StringSlice
	var takeover bool
	idleTimeout := defaultDaemonIdleTimeout

	// Keep the bootstrap grant the Device enrolls through, then build the command.
	var deviceGrant string
	return &cli.Command{
		Name:  "serve",
		Usage: "start the daemon and listen for CLI connections",
		Flags: append([]cli.Flag{
			&cli.BoolFlag{
				Name:        "takeover",
				Usage:       "ask any existing runtime on the socket to yield",
				Destination: &takeover,
			},
			&cli.DurationFlag{
				Name:        "idle-timeout",
				Usage:       "duration controls shutdown after the last active client/service; zero disables idle shutdown",
				Value:       defaultDaemonIdleTimeout,
				Destination: &idleTimeout,
			},
			&cli.StringFlag{
				Name:        "daemon-startup-pipe-id",
				Usage:       "internal startup pipe identifier",
				Destination: &startupPipeID,
				Hidden:      true,
			},
			&cli.StringFlag{
				Name:        "daemon-launcher",
				Usage:       "internal record of what started the daemon",
				Destination: &launcher,
				Hidden:      true,
			},
			&cli.StringFlag{
				Name:        "trace",
				Usage:       "write a Go runtime trace for the daemon process",
				EnvVars:     []string{daemon.TracePathEnvVar},
				Destination: &runtimeTracePath,
			},
			&cli.StringFlag{
				Name:        "udp-listen",
				Usage:       "serve other sessions of the account over UDP on this host:port; disables idle shutdown unless --idle-timeout is set",
				Destination: &udpListen,
			},
			&cli.StringSliceFlag{
				Name: "udp-peer",
				Usage: "admit links from this session peer ID on the --udp-listen socket, such as a Space member joining by invite; " +
					"ID@HOST:PORT also dials it and holds the link, a test path for a LAN without signaling (repeatable)",
				Destination: &udpPeers,
			},
			&cli.StringFlag{
				Name:        "device-grant",
				Usage:       "enroll this Device in a Space through the bootstrap grant at this path, such as a Spacewave OS image's grant",
				Destination: &deviceGrant,
			},
		}, append(append(frees.BuildFlags(), upgrade.BuildFlags()...), serveColdPackFlags()...)...),
		Action: func(c *cli.Context) (retErr error) {
			return runWithRuntimeTrace(runtimeTracePath, func() error {
				return runServeCommand(
					c,
					getBus,
					yieldBroker,
					startupPipeID,
					daemon.Launcher(launcher),
					takeover,
					idleTimeout,
					udpListen,
					udpPeers.Value(),
					deviceGrant,
					frees,
					devicePolicy,
				)
			})
		},
	}
}

// runServeCommand owns the state lease, Resource readiness and common daemon
// lifetime. A manual start asks a daemon started for a command to yield.
func runServeCommand(
	c *cli.Context,
	getBus func() cli_entrypoint.CliBus,
	yieldBroker *yield_policy.Broker,
	startupPipeID string,
	launcher daemon.Launcher,
	takeover bool,
	idleTimeout time.Duration,
	udpListen string,
	udpPeers []string,
	deviceGrant string,
	frees *CollectorFrees,
	devicePolicy *device_policy.PolicyStore,
) (retErr error) {
	// A service manager's SIGTERM, or an interrupt, stops serve from outside:
	// clients drain and the bus releases the state lease, at any point from
	// startup on. A second signal kills the process.
	ctx, cancel := context.WithCancel(c.Context)
	defer cancel()
	stopOnSignal(ctx, cancel)

	// Resolve the command target before any writable bus is requested.
	resolved, err := resolveStatePathFromContext(c, "")
	if err != nil {
		return err
	}
	startupNotifier, err := daemon.NewStartupNotifier(ctx, resolved, startupPipeID)
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			startupNotifier.Error(retErr)
			return
		}
		startupNotifier.Close()
	}()
	if !c.IsSet("idle-timeout") {
		idleTimeout, err = getDaemonIdleTimeout()
		if err != nil {
			return err
		}

		// Remote sessions are not local clients, so a listener stays up.
		if udpListen != "" {
			idleTimeout = 0
		}
	}

	// Suppress the bus-hosted listener while serve owns the public socket.
	sockPath := serveSocketPath(c, resolved)

	// Pass the exact daemon socket to this daemon's desktop descendants.
	previousDesktopSocket, hadDesktopSocket := os.LookupEnv(desktopDaemonSocketEnvVar)
	if err := os.Setenv(desktopDaemonSocketEnvVar, sockPath); err != nil {
		return errors.Wrap(err, "expose desktop daemon socket to child")
	}
	defer func() {
		var restoreErr error
		if hadDesktopSocket {
			restoreErr = os.Setenv(desktopDaemonSocketEnvVar, previousDesktopSocket)
		} else {
			restoreErr = os.Unsetenv(desktopDaemonSocketEnvVar)
		}
		if restoreErr != nil && retErr == nil {
			retErr = errors.Wrap(restoreErr, "restore desktop daemon socket environment")
		}
	}()
	handoffBroker := yieldBroker
	handoffBroker.BeginHandoff("spacewave serve", sockPath)
	defer handoffBroker.Reclaim()

	// Hold exclusion through bus teardown, including initialization failures.
	claim := claimFree
	switch {
	case takeover:
		claim = claimTakeover
	case startupPipeID == "":
		claim = claimYield
	}
	statePathLease, err := prepareDaemonRuntime(ctx, nil, resolved, sockPath, claim)
	if err != nil {
		return err
	}
	leaseOwned := true
	defer func() {
		if leaseOwned {
			if err := statePathLease.release(); err != nil && retErr == nil {
				retErr = errors.Wrap(err, "release writable state path lease")
			}
		}
	}()

	// Load the Device policy before the bus starts a plugin that asks it.
	if err := devicePolicy.Load(resolved); err != nil {
		return err
	}

	// Register the cold configuration before the bus starts the local provider.
	releaseColdPacks, err := configureServeColdPacks(c)
	if err != nil {
		return err
	}

	// Transfer lease and cold configuration cleanup after bus construction.
	cliBus := getBus()
	if cliBus == nil {
		if releaseColdPacks != nil {
			releaseColdPacks()
		}
		return errors.New("bus not initialized")
	}
	if releaseColdPacks != nil {
		cliBus.AddRelease(releaseColdPacks)
	}
	le := cliBus.GetLogger()
	cliBus.AddRelease(func() {
		if err := statePathLease.release(); err != nil {
			le.WithError(err).Error("failed to release writable state path lease")
		}
	})
	leaseOwned = false
	setServeMemoryLimit(le)

	// Log the Volumes that free, refusing a name no Volume file has.
	if err := frees.Check(le, cliBus.GetStateRoot()); err != nil {
		return err
	}
	serveCtx, serveCancel := context.WithCancel(ctx)
	defer serveCancel()

	// Native core runs on the CLI bus. Dist core runs in the spacewave-core
	// plugin process.
	nativeCore := cliBus.GetPluginHostObjectKey() == ""
	if releaseColdPacks != nil && !nativeCore {
		return errors.New("cold packs require a native core in the same process as the Unix control socket")
	}
	invoker := newDaemonResourceInvoker(cliBus.GetBus())
	if nativeCore {
		var invokerRef directive.Reference
		invoker, invokerRef, err = lookupLocalResourceInvoker(serveCtx, cliBus.GetBus())
		if err != nil {
			return err
		}
		defer invokerRef.Release()

		// Native core owns the same Resource authority as the core plugin.
		// Present native core as that plugin before starting plugins
		// that register types.
		releaseCorePlugin, err := cliBus.GetBus().AddController(serveCtx, newNativeCorePlugin(cliBus.GetBus(), invoker), nil)
		if err != nil {
			return err
		}
		defer releaseCorePlugin()
	}

	// Native hosts supply the same plugin-host services as a distribution bus.
	if nativeCore {
		pluginRoot := filepath.Join(resolved, "plugin")
		pluginStateRoot := filepath.Join(pluginRoot, "state")
		pluginDistRoot := filepath.Join(pluginRoot, "dist")
		for _, dir := range []string{pluginStateRoot, pluginDistRoot} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		_, release, err := plugin_host_default.StartPluginHost(
			serveCtx,
			cliBus.GetBus(),
			pluginStateRoot,
			pluginDistRoot,
			"",
		)
		if err != nil {
			return err
		}
		defer release()

		// Serve the files of local app builds imported into the daemon
		// World, for bound web listeners. The scheduler leaves every other
		// plugin to native core and runs nothing: both host platforms deny
		// the app. Imported blocks are already in the daemon volume.
		vol := cliBus.GetVolume()
		schedConf := plugin_host_default.NewSchedulerConfig(
			"",
			cliBus.GetWorldEngineID(),
			defaultPluginHostObjectKey,
			vol.GetID(),
			vol.GetPeerID().String(),
			false, // No remote catalog fetches manifests.
			false, // Keep the World store as the only manifest source.
			true,  // Do not copy manifests the volume already holds.
		)
		schedConf.PluginIds = slices.Clone(resource_root.BoundPluginIDs)
		for _, platformID := range []string{
			bldr_platform.NewJsPlatform().GetPlatformID(),
			(&bldr_platform.NativePlatform{}).GetPlatformID(),
		} {
			schedConf.PlatformSelectionPolicies = append(schedConf.PlatformSelectionPolicies, &plugin_host_scheduler.PlatformSelectionPolicy{
				PlatformId:      platformID,
				DeniedPluginIds: schedConf.PluginIds,
			})
		}
		_, _, schedRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_scheduler.Controller](
			serveCtx,
			cliBus.GetBus(),
			resolver.NewLoadControllerWithConfig(schedConf),
			nil,
		)
		if err != nil {
			return err
		}
		defer schedRef.Release()
	}

	// Tie background projections and remote services to the serving lifetime.
	// Find the native plugin host root on the bus, whichever host serves it.
	nativeHostRoot, _, hostRef, err := plugin_host_root.ExLookupRootByPlatform(
		serveCtx, cliBus.GetBus(), false, (&bldr_platform.NativePlatform{}).GetPlatformID(), nil,
	)
	if err != nil {
		return err
	}
	defer hostRef.Release()

	// Give the host root the Forge Worker declaration the Reconciler accepts.
	forgeWorker := device_flowgraph.NewForgeWorkerWatch()
	nativeHostRoot.SetForgeWorkerSource(forgeWorker)
	if nativeCore {
		// Native core reaches the same host Resource service on its local bus.
		pluginRoot := plugin_host_resource.NewPluginHostRoot(
			cliBus.GetBus(), nativeCorePluginID, "", nil, nil, nil, nativeHostRoot,
			"native-policy-atoms", cliBus.GetVolume().GetID(), nil,
		)
		defer pluginRoot.Release()
		hostMux := srpc.NewMux()
		if err := resource_server.NewResourceServer(pluginRoot.GetMux(), "").Register(hostMux); err != nil {
			return err
		}
		hostCtrl := bifrost_rpc.NewInvokerController(le, cliBus.GetBus(),
			controller.NewInfo("cli/native-core-policy-host", controller.MustParseVersion("0.0.1"), ""),
			hostMux, []string{bldr_plugin.HostServiceIDPrefix},
		)
		releaseHost, err := cliBus.GetBus().AddController(serveCtx, hostCtrl, nil)
		if err != nil {
			return err
		}
		defer releaseHost()
	}

	// Project the daemon's Device state into the Space.
	startLocalSessionKeeper(serveCtx, le, resolved, deviceGrant, invoker)
	startDeviceUpdateProjection(serveCtx, le, resolved, cliBus.GetBus(), invoker)

	// Serve the core Flowgraph node types.
	releaseFlowgraphNodeTypes, err := cliBus.GetBus().AddController(serveCtx, flowgraph_nodetype.NewController(), nil)
	if err != nil {
		return err
	}
	defer releaseFlowgraphNodeTypes()

	// Protect and bind the Resource socket before publishing daemon readiness.
	explicitSocket := effectiveSocketPath(c, "") != ""
	lis, err := resource_listener.ListenProtectedUnix(sockPath, !explicitSocket)
	if err != nil {
		return errors.Wrapf(err, "listen on daemon socket %s", sockPath)
	}
	defer lis.Close()

	// Retain startup demand until readiness. Then every daemon uses idle
	// expiry.
	le.Infof("listening on %s", sockPath)
	idleTracker := newDaemonIdleTracker(idleTimeout, func() {
		le.Info("daemon idle timeout reached, shutting down")
		serveCancel()
		lis.Close()
	})
	defer idleTracker.close()
	releaseStartupDemand := idleTracker.serviceAttached("daemon startup")
	defer releaseStartupDemand()

	// Persistent services and pending uploads participate in the same idle
	// count as public clients.
	startWebListenerKeepalive(serveCtx, le, invoker, idleTracker)
	startSyncKeepalive(serveCtx, le, invoker, idleTracker)
	startFlowgraphReconciler(serveCtx, le, resolved, cliBus.GetBus(), invoker, devicePolicy, forgeWorker, idleTracker)

	// Retain desktop demand independently of the connection that opens it.
	// The os.system services route to a placed os/system node, for the
	// console of a Device running Spacewave OS.
	// A requested shutdown stops admission by closing the listener. A daemon
	// started for a command also yields to a manual start.
	controlHandler := newDaemonControlHandler(func() { _ = lis.Close() })
	if launcher == daemon.LauncherCommand {
		controlHandler.SetYieldPolicy(listener_control.AutoAllowPolicy)
	}

	// Route desktop Quit through the same shutdown request.
	mux := srpc.NewMux(invoker, os_system.NewInvoker(cliBus.GetBus()))
	desktopControl := newDaemonDesktopControl(serveCtx, cliBus.GetBus(), idleTracker, controlHandler.requestDesktopQuit)
	defer desktopControl.close()
	if err := desktop_control.SRPCRegisterDesktopControlService(mux, desktopControl); err != nil {
		return err
	}

	// Register local controls before allowing clients onto the listener.
	if err := mux.Register(controlHandler); err != nil {
		return err
	}
	if err := device_policy.SRPCRegisterDevicePolicyService(mux, device_policy.NewService(devicePolicy)); err != nil {
		return err
	}
	if releaseColdPacks != nil {
		if err := mux.Register(coldPackControlHandler{}); err != nil {
			return err
		}
	}
	if err := s4wave_trace.SRPCRegisterTraceService(mux, trace_service.NewService()); err != nil {
		return err
	}

	// Verify the actual native or forwarded Resource service before handing
	// custody to the launcher. Public accepts begin only after acknowledgement.
	readyClient, err := buildSDKClientFromInvoker(serveCtx, mux)
	if err != nil {
		return errors.Wrap(err, "initialize daemon Resource service")
	}
	defer readyClient.close()
	if err := startupNotifier.Ready(serveCtx); err != nil {
		return err
	}

	// Wake lease losers even when they consumed the socket's bind event before
	// listen completed. Both core shapes publish the same post-custody event.
	if err := daemon.PublishReady(sockPath); err != nil {
		return errors.Wrap(err, "publish daemon readiness")
	}

	// Public service starts only after the launcher relinquishes custody.
	srv := srpc.NewServer(mux)
	releaseStartupDemand()
	if udpListen != "" {
		go func() {
			if err := serveUDPListener(serveCtx, le, readyClient, udpListen, udpPeers); err != nil {
				le.WithError(err).Error("UDP listener stopped")
			}
		}()
	}
	updatePath := make(chan *daemonUpdateHandoff, 1)
	go func() {
		handoff, err := watchDaemonUpdate(serveCtx, le, cliBus.GetBus(), resolved, idleTracker, desktopControl)
		if err != nil {
			if serveCtx.Err() == nil {
				le.WithError(err).Warn("daemon update watch ended")
			}
			return
		}
		updatePath <- handoff
		_ = lis.Close()
	}()

	// Drain the claimed idle runtime before bus cleanup releases its state lease.
	err = serveDaemonListener(serveCtx, serveCancel, lis, srv, controlHandler, idleTracker)
	if err != nil {
		return err
	}
	select {
	case handoff := <-updatePath:
		// Both bus implementations run caller cleanup in registration
		// order, so the earlier lease release precedes this readiness
		// handoff.
		cliBus.AddRelease(func() {
			// Start the selected daemon and reopen the desktop when requested.
			le.Info("old daemon state lease released; starting selected daemon executable")
			startCtx := context.WithoutCancel(ctx)
			if err := daemon.StartExecutable(startCtx, resolved, handoff.selected, launcher); err != nil {
				le.WithError(err).Error("updated daemon did not become ready; restoring previous executable")
				if fallbackErr := daemon.StartExecutable(startCtx, resolved, handoff.fallback, launcher); fallbackErr != nil {
					le.WithError(errors.Wrapf(fallbackErr, "updated daemon failed: %v", err)).Error("failed to restore previous daemon")
					return
				}
			}
			if handoff.reopen == nil {
				return
			}
			if err := reopenDesktop(startCtx, resolved, handoff.reopen); err != nil {
				le.WithError(err).Warn("could not reopen the desktop on the replacement daemon")
			}
		})
	default:
	}
	return nil
}

// lookupLocalResourceInvoker waits for the Resource service already registered
// on the CLI bus and returns the reference that keeps it alive.
func lookupLocalResourceInvoker(
	ctx context.Context,
	b bus.Bus,
) (srpc.Invoker, directive.Reference, error) {
	// Retain the local Resource service directive for the daemon's lifetime.
	invokers, _, invokerRef, err := bifrost_rpc.ExLookupRpcService(
		ctx,
		b,
		resource.SRPCResourceServiceServiceID,
		"",
		true,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	if len(invokers) == 0 {
		return nil, nil, errors.New("resource service not found")
	}
	return invokers[0], invokerRef, nil
}

// daemonPluginClientLoader waits for the current spacewave-core generation.
type daemonPluginClientLoader func(context.Context) (srpc.Client, directive.Reference, error)

// daemonResourceInvoker forwards each stream to the current core plugin.
type daemonResourceInvoker struct {
	// loadClient acquires the plugin client and its stream-scoped reference.
	loadClient daemonPluginClientLoader
}

// newDaemonResourceInvoker routes each Resource RPC stream to the current
// spacewave-core plugin generation.
func newDaemonResourceInvoker(b bus.Bus) srpc.Invoker {
	return &daemonResourceInvoker{
		loadClient: func(ctx context.Context) (srpc.Client, directive.Reference, error) {
			return bldr_plugin.ExPluginLoadWaitClient(ctx, b, "spacewave-core", nil)
		},
	}
}

// InvokeMethod retains the selected core generation until the stream ends.
func (i *daemonResourceInvoker) InvokeMethod(
	serviceID, methodID string,
	strm srpc.Stream,
) (bool, error) {
	// Leave non-Resource services to the daemon's local control handlers.
	if serviceID != resource.SRPCResourceServiceServiceID {
		return false, nil
	}

	// Keep the selected plugin alive until this forwarded stream completes.
	client, clientRef, err := i.loadClient(strm.Context())
	if err != nil || clientRef == nil {
		return false, err
	}
	defer clientRef.Release()
	return srpc.NewClientInvoker(client).InvokeMethod(serviceID, methodID, strm)
}

// stopOnSignal calls stop on the first SIGTERM or interrupt before ctx ends,
// then restores the signals' default action.
func stopOnSignal(ctx context.Context, stop func()) {
	// Subscribe before returning so a signal sent right after cannot kill
	// the process.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, os.Interrupt)
	go func() {
		defer signal.Stop(signals)
		select {
		case <-signals:
			stop()
		case <-ctx.Done():
		}
	}()
}

// _ is a type assertion.
var _ srpc.Invoker = (*daemonResourceInvoker)(nil)
